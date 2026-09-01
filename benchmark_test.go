package lumberjack

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// BenchmarkBufferedLoggerWrite measures end-to-end batching, including every
// Flush to Logger and therefore the underlying filesystem writes.
func BenchmarkBufferedLoggerWrite(b *testing.B) {
	const writesPerFlush = 157 // 157 * 26 bytes fits in a 4 KiB buffer.
	l := &Logger{
		Filename: filepath.Join(b.TempDir(), "benchmark.log"),
		MaxSize:  1024,
	}
	w := bufio.NewWriterSize(l, 4096)
	b.Cleanup(func() {
		_ = w.Flush()
		_ = l.Close()
	})

	p := []byte("a representative log line\n")
	b.ReportAllocs()
	b.SetBytes(int64(writesPerFlush * len(p)))
	b.ResetTimer()
	for b.Loop() {
		for range writesPerFlush {
			if _, err := w.Write(p); err != nil {
				b.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoggerWriteSizes(b *testing.B) {
	for _, size := range []int{26, 256, 4096} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			l := &Logger{
				Filename: filepath.Join(b.TempDir(), "benchmark.log"),
				MaxSize:  1024,
			}
			b.Cleanup(func() { _ = l.Close() })
			p := make([]byte, size)
			if _, err := l.Write(p); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for b.Loop() {
				if _, err := l.Write(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFileRemovalCheck(b *testing.B) {
	f, err := os.OpenFile(filepath.Join(b.TempDir(), "benchmark.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = f.Close() })
	b.ReportAllocs()
	for b.Loop() {
		if fileWasRemoved(f, nil) {
			b.Fatal("benchmark file was unexpectedly removed")
		}
	}
}

// BenchmarkLoggerWrite measures the steady-state logging path after the file
// has been opened. It intentionally excludes rotation and setup work.
func BenchmarkLoggerWrite(b *testing.B) {
	l := &Logger{
		Filename: filepath.Join(b.TempDir(), "benchmark.log"),
		MaxSize:  1024,
	}
	b.Cleanup(func() { _ = l.Close() })

	p := []byte("a representative log line\n")
	if _, err := l.Write(p); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(p)))
	b.ResetTimer()
	for b.Loop() {
		if _, err := l.Write(p); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMillDisabled measures the default configuration, where there is no
// retention or compression work to perform. Production applications may
// create a Logger per component or recreate them during configuration reloads.
func BenchmarkMillDisabled(b *testing.B) {
	loggers := make([]Logger, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range loggers {
		loggers[i].mill()
	}
	// Keep the loggers (and their channels) alive through the measurement.
	runtime.KeepAlive(loggers)
}

// BenchmarkFileWrite provides the lower bound imposed by os.File itself.
func BenchmarkFileWrite(b *testing.B) {
	f, err := os.OpenFile(filepath.Join(b.TempDir(), "benchmark.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = f.Close() })

	p := []byte("a representative log line\n")
	b.ReportAllocs()
	b.SetBytes(int64(len(p)))
	b.ResetTimer()
	for b.Loop() {
		if _, err := io.Writer(f).Write(p); err != nil {
			b.Fatal(err)
		}
	}
}
