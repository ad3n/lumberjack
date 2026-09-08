package lumberjack

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Concurrent callers have no defined global arrival order. Each caller's
// completed Writes must retain its order, including across automatic rotation.
func TestConcurrentWriteOrderAcrossRotation(t *testing.T) {
	const workers, records = 8, 1024
	l := &Logger{Filename: filepath.Join(t.TempDir(), "order.log"), MaxSize: 1}
	t.Cleanup(func() { _ = l.Close() })
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for seq := range records {
				line := []byte(fmt.Sprintf("%02d %04d %0247d\n", worker, seq, 0))
				if n, err := l.Write(line); err != nil || n != len(line) {
					errs <- fmt.Errorf("Write = %d, %v", n, err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err := l.oldLogFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Fatal("expected automatic rotation")
	}
	var names []string
	for i := len(backups) - 1; i >= 0; i-- {
		names = append(names, filepath.Join(l.dir(), backups[i].Name()))
	}
	names = append(names, l.Filename)
	next := make([]int, workers)
	for _, name := range names {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		var readErr error
		for scanner.Scan() {
			var worker, seq int
			if _, err := fmt.Sscanf(scanner.Text(), "%d %d", &worker, &seq); err != nil {
				readErr = err
				break
			}
			if worker < 0 || worker >= workers || seq != next[worker] {
				readErr = fmt.Errorf("out-of-order/duplicate record: %d %d; next=%v", worker, seq, next)
				break
			}
			next[worker]++
		}
		scanErr := scanner.Err()
		closeErr := f.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	for worker, count := range next {
		if count != records {
			t.Fatalf("worker %d: got %d records, want %d", worker, count, records)
		}
	}
}
