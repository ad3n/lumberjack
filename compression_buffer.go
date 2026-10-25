package lumberjack

import (
	"io"
	"sync"
)

var compressionBuffers compressionBufferPool

type (
	compressionBufferPool struct {
		buffers [4]*compressionBuffer
		mu      sync.Mutex
	}

	compressionBuffer struct {
		reader io.Reader
		data   *[32 * 1024]byte
	}
)

func (p *compressionBufferPool) get() *compressionBuffer {
	p.mu.Lock()
	for i, buffer := range p.buffers {
		if buffer == nil {
			continue
		}

		p.buffers[i] = nil
		p.mu.Unlock()
		return buffer
	}

	p.mu.Unlock()
	return &compressionBuffer{data: new([32 * 1024]byte)}
}

func (p *compressionBufferPool) put(buffer *compressionBuffer) {
	buffer.reader = nil
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.buffers {
		if p.buffers[i] != nil {
			continue
		}

		p.buffers[i] = buffer
		return
	}
}

func (p *compressionBufferPool) copy(dst io.Writer, src io.Reader) (int64, error) {
	buffer := p.get()
	defer p.put(buffer)

	buffer.reader = src
	return io.CopyBuffer(dst, buffer, buffer.data[:])
}

func (b *compressionBuffer) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}
