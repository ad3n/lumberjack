package lumberjack

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
)

type (
	compressionTestReader struct {
		data   []byte
		err    error
		closed bool
	}

	compressionShortWriter struct{}
)

func (r *compressionTestReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func (r *compressionTestReader) Close() error {
	r.closed = true
	return nil
}

func (r *compressionTestReader) WriteTo(io.Writer) (int64, error) {
	panic("copy bypassed pooled buffer")
}

func (compressionShortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func TestCompressionBufferCopy(t *testing.T) {
	for _, stage := range []string{"success", "read", "write", "short-write"} {
		t.Run(stage, func(t *testing.T) {
			var pool compressionBufferPool
			buffer := pool.get()
			pool.put(buffer)
			for range 3 {
				src := &compressionTestReader{data: []byte("payload"), err: io.EOF}
				dst := &poolTestWriter{}
				var target io.Writer = struct{ io.Writer }{dst}
				var wantErr error
				wantN := int64(7)
				switch stage {
				case "read":
					src.err = io.ErrUnexpectedEOF
					wantErr = io.ErrUnexpectedEOF
				case "write":
					dst.fail = true
					wantErr = io.ErrClosedPipe
					wantN = 0
				case "short-write":
					target = compressionShortWriter{}
					wantErr = io.ErrShortWrite
					wantN = 6
				}

				n, err := pool.copy(target, src)
				if n != wantN || !errors.Is(err, wantErr) {
					t.Fatalf("copy = %d, %v; want %d, %v", n, err, wantN, wantErr)
				}

				if src.closed || dst.closed {
					t.Fatal("copy closed caller resource")
				}

				if pool.buffers[0] != buffer || buffer.reader != nil {
					t.Fatal("copy did not return buffer")
				}

				var next bytes.Buffer
				data := bytes.Repeat([]byte("next payload"), 10000)
				n, err = pool.copy(&next, bytes.NewReader(data))
				if err != nil || n != int64(len(data)) || !bytes.Equal(next.Bytes(), data) {
					t.Fatalf("reuse after %s failed: %d, %v", stage, n, err)
				}
			}
		})
	}
}

func TestCompressionBufferCapacity(t *testing.T) {
	var pool compressionBufferPool
	var active [8]*compressionBuffer
	for i := range active {
		active[i] = pool.get()
		for j := range i {
			if active[i] == active[j] {
				t.Fatal("active buffer shared")
			}
		}
	}

	for _, buffer := range active {
		pool.put(buffer)
	}

	for i, buffer := range pool.buffers {
		if buffer != active[i] {
			t.Fatal("bounded pool did not retain expected buffer")
		}
	}
}

func TestCompressionBufferConcurrent(t *testing.T) {
	var pool compressionBufferPool
	var wg sync.WaitGroup
	for worker := range 12 {
		wg.Go(func() {
			data := bytes.Repeat([]byte{byte(worker)}, 100000)
			for range 20 {
				var dst bytes.Buffer
				n, err := pool.copy(&dst, bytes.NewReader(data))
				if err != nil || n != int64(len(data)) || !bytes.Equal(dst.Bytes(), data) {
					t.Error("concurrent copy corrupted data", n, err)
				}
			}
		})
	}

	wg.Wait()
}

func TestCompressionCopyZeroAllocations(t *testing.T) {
	var pool compressionBufferPool
	data := bytes.Repeat([]byte("payload"), 10000)
	src := bytes.NewReader(data)
	dst := &struct{ io.Writer }{io.Discard}
	allocations := testing.AllocsPerRun(100, func() {
		src.Reset(data)
		n, err := pool.copy(dst, src)
		if err != nil || n != int64(len(data)) {
			t.Fatalf("copy = %d, %v", n, err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed copy allocated %g times", allocations)
	}
}
