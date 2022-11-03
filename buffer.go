package iorpc

import (
	"bytes"
	"io"
	"sync"
)

func RegisterAllocator(allocator func(size int) Buffer) {
	bufferAllocator = allocator
}

var bufferAllocator func(size int) Buffer

func init() {
	if bufferAllocator == nil {
		RegisterAllocator(func(size int) Buffer {
			buf := bufferPool.Get().(*buffer)
			buf.Grow(size)
			return buf
		})
	}
}

type Buffer interface {
	io.ReadWriteCloser
	Reset()
	Bytes() []byte
	Underlying() any
}

var bufferPool = sync.Pool{
	New: func() any {
		return &buffer{
			bytes.NewBuffer(make([]byte, 0)),
		}
	},
}

type buffer struct {
	*bytes.Buffer
}

func (b *buffer) Close() error {
	b.Reset()
	bufferPool.Put(b)
	return nil
}

func (b *buffer) Underlying() any {
	return b.Buffer
}
