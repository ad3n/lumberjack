package lumberjack

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkRotateFile(b *testing.B) {
	savedTime := currentTime
	currentTime = func() time.Time { return time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) }
	defer func() { currentTime = savedTime }()

	for _, compress := range []bool{false, true} {
		b.Run(fmt.Sprintf("compress_%t", compress), func(b *testing.B) {
			l := &Logger{Filename: filepath.Join(b.TempDir(), "rotate.log"), Compress: compress}
			b.Cleanup(func() { _ = l.Close() })
			backup := backupName(l.Filename, false)
			if compress {
				backup += compressSuffix
			}

			data := bytes.Repeat([]byte("log record\n"), 4096)
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if _, err := l.Write(data); err != nil {
					b.Fatal(err)
				}

				if err := l.Rotate(); err != nil {
					b.Fatal(err)
				}

				if err := l.Close(); err != nil {
					b.Fatal(err)
				}

				if err := os.Remove(backup); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
