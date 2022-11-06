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

	// Bytes returns the written byte slice.
	// The slice must be sequential.
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

type ringBuffer struct {
	start, size int64
	buf         []byte
}

func newRingBuffer(cap int) *ringBuffer {
	return &ringBuffer{
		buf: make([]byte, cap),
	}
}

func (b *ringBuffer) Size() int64 {
	return b.size
}

func (b *ringBuffer) Capacity() int64 {
	return int64(len(b.buf))
}

func (b *ringBuffer) Close() error {
	return nil
}

func (b *ringBuffer) Reset() {
	b.start, b.size = 0, 0
}

func (b *ringBuffer) Produce(producer func() (int, error)) (int, error) {
	n, err := producer()
	if err != nil {
		return n, err
	}
	b.size += int64(n)
	return n, nil
}

func (b *ringBuffer) Consume(consumer func() (int, error)) (int, error) {
	n, err := consumer()
	if err != nil {
		return n, err
	}
	b.start += int64(n)
	b.size -= int64(n)
	return n, nil
}

func (b *ringBuffer) Read(p []byte) (int, error) {
	return b.Consume(func() (int, error) {
		return b.read(p)
	})
}

func (b *ringBuffer) read(p []byte) (int, error) {
	if b.size == 0 {
		return 0, io.EOF
	}

	index := b.start % b.Capacity()
	if index+b.size <= b.Capacity() {
		return copy(p, b.buf[index:index+b.size]), nil
	}

	n := copy(p, b.buf[b.start:])
	if len(p) > n {
		n += copy(p[n:], b.buf[:(index+b.size)%b.Capacity()])
	}
	return n, nil
}

func (b *ringBuffer) TrySlice(size int64) (data []byte, sliced bool) {
	data, sliced = b.trySlice(size)
	b.start += int64(len(data))
	b.size -= int64(len(data))
	return data, sliced
}

func (b *ringBuffer) trySlice(size int64) (data []byte, sliced bool) {
	if b.size == 0 {
		return nil, false
	}

	index := b.start % b.Capacity()

	if b.size < size {
		size = b.size
	}

	if index+size <= b.Capacity() {
		return b.buf[index : index+size], true
	}

	data = make([]byte, size)
	n, _ := b.read(data)
	return data[:n], false
}

func (b *ringBuffer) Write(p []byte) (int, error) {
	return b.Produce(func() (int, error) {
		return b.write(p)
	})
}

func (b *ringBuffer) write(p []byte) (int, error) {
	if b.size+int64(len(p)) > b.Capacity() {
		data, _ := b.trySlice(b.size)
		b.buf = append(data, p...)
		b.start = 0
		return len(p), nil
	}

	index := b.start % b.Capacity()

	n := copy(b.buf[index:], p)
	if len(p) > n {
		n += copy(b.buf, p[n:])
	}
	return n, nil
}
