package lumberjack

import (
	"bytes"
	"io"
	"testing"
)

func BenchmarkCompressionCopy(b *testing.B) {
	var pool compressionBufferPool
	data := bytes.Repeat([]byte("log record\n"), 6554)
	src := bytes.NewReader(data)
	dst := &struct{ io.Writer }{io.Discard}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		src.Reset(data)
		if _, err := pool.copy(dst, src); err != nil {
			b.Fatal(err)
		}
	}
}
