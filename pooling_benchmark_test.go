package lumberjack

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCompressFile(b *testing.B) {
	for _, size := range []int{64 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			dir := b.TempDir()
			src := filepath.Join(dir, "backup.log")
			dst := src + compressSuffix
			data := bytes.Repeat([]byte("representative log record with timestamp and request details\n"), size/60+1)[:size]
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				if err := os.WriteFile(src, data, 0600); err != nil {
					b.Fatal(err)
				}

				if err := compressLogFile(src, dst); err != nil {
					b.Fatal(err)
				}

				if err := os.Remove(dst); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
