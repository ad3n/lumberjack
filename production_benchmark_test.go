package lumberjack

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

// BenchmarkProductionFile measures complete batches reaching real files, with
// automatic size rotation and a final close included. Buffering is a separate,
// explicitly opt-in workload: its lock covers a batch and Flush, since bufio
// itself is not safe for concurrent use. No data is left queued at measurement end.
// Retention/compression and fsync are excluded; Logger.Write does not fsync.
func BenchmarkProductionFile(b *testing.B) {
	for _, workers := range []int{1, 8, 32} {
		for _, buffered := range []bool{false, true} {
			b.Run(fmt.Sprintf("workers_%d/buffered_%t", workers, buffered), func(b *testing.B) {
				const records = 128
				const size = 256
				dir := b.TempDir()
				l := &Logger{Filename: filepath.Join(dir, "production.log"), MaxSize: 16}
				b.Cleanup(func() { _ = l.Close() })
				p := make([]byte, size)
				for i := range p {
					p[i] = ' '
				}
				copy(p, `{"level":"info","service":"api","method":"GET","path":"/orders","status":200,"request_id":"0123456789abcdef","message":"request completed"}`)
				p[len(p)-1] = '\n'
				w := bufio.NewWriterSize(l, 32*1024)
				var mu sync.Mutex
				// At most 4096 latency samples per worker, independent of b.N.
				samples := make([][]int64, workers)
				errs := make([]error, workers)
				var wg sync.WaitGroup
				start := make(chan struct{})
				for worker := range workers {
					samples[worker] = make([]int64, 0, 4096)
					wg.Add(1)
					go func(worker int) {
						defer wg.Done()
						<-start
						count := b.N / workers
						if worker < b.N%workers {
							count++
						}
						stride := max(1, (count+4095)/4096)
						for i := range count {
							var began time.Time
							if i%stride == 0 {
								began = time.Now()
							}
							if buffered {
								mu.Lock()
							}
							var err error
							for range records {
								if buffered {
									_, err = w.Write(p)
								} else {
									_, err = l.Write(p)
								}
								if err != nil {
									break
								}
							}
							if buffered {
								if err == nil {
									err = w.Flush()
								}
								mu.Unlock()
							}
							if err != nil {
								errs[worker] = err
								return
							}
							if !began.IsZero() {
								samples[worker] = append(samples[worker], time.Since(began).Nanoseconds())
							}
						}
					}(worker)
				}
				b.ReportAllocs()
				b.SetBytes(records * size)
				b.ResetTimer()
				close(start)
				wg.Wait()
				err := l.Close()
				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}
				for _, err := range errs {
					if err != nil {
						b.Fatal(err)
					}
				}
				var latencies []int64
				for _, sample := range samples {
					latencies = append(latencies, sample...)
				}
				sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
				if len(latencies) > 0 {
					b.ReportMetric(float64(latencies[(len(latencies)-1)*99/100]), "p99-batch-ns")
				}
				// Check all bytes survived rotations; timestamp collisions would fail this.
				files, err := os.ReadDir(dir)
				if err != nil {
					b.Fatal(err)
				}
				var total int64
				for _, file := range files {
					info, err := file.Info()
					if err != nil {
						b.Fatal(err)
					}
					total += info.Size()
				}
				if want := int64(b.N) * records * size; total != want {
					b.Fatalf("bytes on disk = %d, want %d", total, want)
				}
			})
		}
	}
}
