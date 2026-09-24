package lumberjack

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type poolTestWriter struct {
	bytes.Buffer
	fail   bool
	closed bool
}

func (w *poolTestWriter) Write(b []byte) (int, error) {
	if w.fail {
		return 0, io.ErrClosedPipe
	}

	return w.Buffer.Write(b)
}

func (w *poolTestWriter) Close() error {
	w.closed = true

	return nil
}

func TestCompressionPoolReuse(t *testing.T) {
	for _, stage := range []string{"success", "write", "flush", "close"} {
		t.Run(stage, func(t *testing.T) {
			var pool compressionPool
			dst := &poolTestWriter{}
			w := pool.get(dst)
			w.Name = "previous.log"
			w.Comment = "previous header"
			if stage == "write" {
				dst.fail = true
			}

			_, err := w.Write([]byte("first payload"))
			if stage == "write" {
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("Write error = %v", err)
				}
			}

			if stage == "flush" {
				dst.fail = true
				if err := w.Flush(); !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("Flush error = %v", err)
				}
			}

			if stage == "close" {
				dst.fail = true
			}

			err = w.Close()
			if stage == "success" && err != nil {
				t.Fatal(err)
			}

			if stage != "success" && !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("Close error = %v", err)
			}

			pool.put(w)
			if dst.closed {
				t.Fatal("caller-owned writer was closed")
			}

			for range 3 {
				var next bytes.Buffer
				reused := pool.get(&next)
				if reused != w || reused.Name != "" || reused.Comment != "" {
					t.Fatal("writer not reused or header not reset")
				}

				if _, err := reused.Write([]byte("next payload")); err != nil {
					t.Fatal(err)
				}

				if err := reused.Close(); err != nil {
					t.Fatal(err)
				}

				pool.put(reused)
				reader, err := gzip.NewReader(&next)
				if err != nil {
					t.Fatal(err)
				}

				got, err := io.ReadAll(reader)
				closeErr := reader.Close()
				if err != nil || closeErr != nil || string(got) != "next payload" {
					t.Fatalf("round trip = %q, %v, %v", got, err, closeErr)
				}
			}
		})
	}
}

func TestCompressionPoolCapacity(t *testing.T) {
	var pool compressionPool
	var active [8]*gzip.Writer
	for i := range active {
		active[i] = pool.get(io.Discard)
		for j := range i {
			if active[i] == active[j] {
				t.Fatal("active writer shared")
			}
		}
	}

	for _, w := range active {
		pool.put(w)
	}

	for _, w := range pool.writers {
		if w == nil {
			t.Fatal("cache slot empty")
		}
	}
}

func TestPooledCompressionFiles(t *testing.T) {
	for worker := range 8 {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := filepath.Join(dir, "source.log")
			dst := src + compressSuffix
			for range 3 {
				if err := os.Mkdir(src, 0700); err != nil {
					t.Fatal(err)
				}

				if err := compressLogFile(src, dst); err == nil {
					t.Fatal("expected directory read failure")
				}

				if _, err := os.Stat(dst); !os.IsNotExist(err) {
					t.Fatalf("partial destination not removed: %v", err)
				}

				if err := os.Remove(src); err != nil {
					t.Fatal(err)
				}

				data := bytes.Repeat([]byte("log record\n"), 1024)
				if err := os.WriteFile(src, data, 0600); err != nil {
					t.Fatal(err)
				}

				if err := compressLogFile(src, dir); err == nil {
					t.Fatal("expected destination open failure")
				}

				if err := compressLogFile(src, dst); err != nil {
					t.Fatal(err)
				}

				if _, err := os.Stat(src); !os.IsNotExist(err) {
					t.Fatalf("source not removed: %v", err)
				}

				compressed, err := os.ReadFile(dst)
				if err != nil {
					t.Fatal(err)
				}

				reader, err := gzip.NewReader(bytes.NewReader(compressed))
				if err != nil {
					t.Fatal(err)
				}

				got, err := io.ReadAll(reader)
				closeErr := reader.Close()
				if err != nil || closeErr != nil || !bytes.Equal(got, data) {
					t.Fatalf("round trip failed: %v, %v", err, closeErr)
				}
			}
		})
	}
}
