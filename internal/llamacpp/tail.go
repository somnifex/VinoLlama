package llamacpp

import "sync"

type TailBuffer struct {
	mu    sync.Mutex
	max   int
	bytes []byte
}

func NewTailBuffer(max int) *TailBuffer {
	if max <= 0 {
		max = 8 * 1024
	}
	return &TailBuffer{max: max}
}

func (b *TailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bytes = append(b.bytes, p...)
	if len(b.bytes) > b.max {
		b.bytes = append([]byte{}, b.bytes[len(b.bytes)-b.max:]...)
	}
	return len(p), nil
}

func (b *TailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte{}, b.bytes...))
}
