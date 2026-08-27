package lumberjack

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

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
