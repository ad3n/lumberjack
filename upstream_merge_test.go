package lumberjack

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUpstreamFileMode(t *testing.T) {
	for _, mode := range []os.FileMode{0, 0600, 0640} {
		t.Run(mode.String(), func(t *testing.T) {
			l := &Logger{Filename: filepath.Join(t.TempDir(), "app.log"), FileMode: mode}
			defer l.Close()

			_, err := l.Write([]byte("before"))
			isNil(err, t)
			info, err := os.Stat(l.Filename)
			isNil(err, t)
			want := mode
			if want == 0 {
				want = 0600
			}

			equals(want, info.Mode().Perm(), t)
			l.FileMode = 0400
			isNil(l.Rotate(), t)
			info, err = os.Stat(l.Filename)
			isNil(err, t)
			equals(os.FileMode(0400), info.Mode().Perm(), t)
			backups, err := l.oldLogFiles()
			isNil(err, t)
			equals(1, len(backups), t)
			old, err := backups[0].Info()
			isNil(err, t)
			equals(want, old.Mode().Perm(), t)
		})
	}
}

func TestUpstreamWorkerCloseAndReuse(t *testing.T) {
	l := &Logger{Filename: filepath.Join(t.TempDir(), "app.log")}
	isNil(l.Rotate(), t)
	if l.millCh != nil {
		t.Fatal("disabled cleanup started worker")
	}

	isNil(l.Close(), t)
	l.Compress = true
	for range 3 {
		_, err := l.Write([]byte("record"))
		isNil(err, t)
		if l.millCh == nil {
			t.Fatal("enabled cleanup did not start worker")
		}

		isNil(l.Rotate(), t)
		isNil(l.Close(), t)
		if l.millCh != nil {
			t.Fatal("Close retained worker channel")
		}

		isNil(l.Close(), t)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 10 {
				if _, err := l.Write([]byte("record")); err != nil {
					t.Error(err)
				}
				if err := l.Close(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	isNil(l.Close(), t)
}

func TestUpstreamCompressionPublish(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.log")
	dst := src + compressSuffix
	data := bytes.Repeat([]byte("log entry\n"), 100)
	isNil(os.WriteFile(src, data, 0600), t)
	isNil(os.Mkdir(dst, 0700), t)
	if err := compressLogFile(src, dst); err == nil {
		t.Fatal("expected rename failure")
	}

	got, err := os.ReadFile(src)
	isNil(err, t)
	if !bytes.Equal(got, data) {
		t.Fatal("source lost on failure")
	}

	if _, err := os.Stat(dst + tmpSuffix); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains: %v", err)
	}

	isNil(os.Remove(dst), t)
	isNil(compressLogFile(src, dst), t)
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source remains: %v", err)
	}

	f, err := os.Open(dst)
	isNil(err, t)
	defer f.Close()

	gz, err := gzip.NewReader(f)
	isNil(err, t)
	defer gz.Close()

	got, err = io.ReadAll(gz)
	isNil(err, t)
	if !bytes.Equal(got, data) {
		t.Fatal("published archive is incomplete")
	}

	if err := compressLogFile(src, dst); err == nil {
		t.Fatal("missing source accepted")
	}

	if _, err := os.Stat(dst); err != nil {
		t.Fatal("repeat failure removed published archive", err)
	}
}

func TestUpstreamWorkerCloseAfterFailure(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{Filename: filepath.Join(dir, "missing", "app.log"), Compress: true}
	l.mu.Lock()
	l.mill()
	worker := l.millCh
	for range 100 {
		l.mill()
	}
	if l.millCh != worker {
		t.Fatal("repeated cleanup replaced worker")
	}
	l.mu.Unlock()
	isNil(l.Close(), t)
	if l.millCh != nil {
		t.Fatal("failed cleanup retained worker")
	}

	_, err := l.Write([]byte("record"))
	isNil(err, t)
	isNil(l.file.Close(), t)
	if err := l.Close(); err == nil {
		t.Fatal("expected file close failure")
	}

	if l.millCh != nil {
		t.Fatal("file close failure retained worker")
	}

	isNil(l.Close(), t)
}
