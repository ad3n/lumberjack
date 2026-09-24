package lumberjack

import (
	"compress/gzip"
	"io"
	"sync"
)

type compressionPool struct {
	writers [4]*gzip.Writer
	mu      sync.Mutex
}

var compressionWriters compressionPool

func (p *compressionPool) get(dst io.Writer) *gzip.Writer {
	p.mu.Lock()
	for i, w := range p.writers {
		if w == nil {
			continue
		}

		p.writers[i] = nil
		p.mu.Unlock()
		w.Reset(dst)

		return w
	}

	p.mu.Unlock()

	return gzip.NewWriter(dst)
}

func (p *compressionPool) put(w *gzip.Writer) {
	w.Reset(io.Discard)
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.writers {
		if p.writers[i] != nil {
			continue
		}

		p.writers[i] = w

		return
	}
}
