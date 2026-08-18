package process

import (
	"bytes"
	"sync"
)

type logBuffer struct {
	mu      sync.Mutex
	data    []byte
	maximum int
}

func newLogBuffer(maximum int) *logBuffer { return &logBuffer{maximum: maximum} }

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.maximum {
		b.data = append([]byte(nil), b.data[len(b.data)-b.maximum:]...)
	}
	return len(p), nil
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(bytes.ToValidUTF8(b.data, []byte("?")))
}
