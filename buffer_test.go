package iorpc

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBuffer(t *testing.T) {
	a := assert.New(t)
	buf := newRingBuffer(0)
	a.Equal(int64(0), buf.Size())
	a.Equal(int64(0), buf.Capacity())

	out := make([]byte, 16)
	n, err := buf.Read(out)
	a.Equal(io.EOF, err)
	a.Equal(0, n)

	data := []byte("hello world")
	n, err = buf.Write(data)
	a.Nil(err)
	a.Equal(len(data), n)
	a.Equal(int64(len(data)), buf.Size())
	a.Equal(int64(len(data)), buf.Capacity())

	n, err = buf.Read(out)
	a.Nil(err)
	a.Equal(len(data), n)
	a.Equal(int64(0), buf.Size())
	a.Equal(int64(len(data)), buf.Capacity())
}

func TestWriteInCapacity(t *testing.T) {
	a := assert.New(t)
	buf := newRingBuffer(1024)
	data := []byte("hello ")
	buf.Write(data)
	data = []byte("world")
	n, err := buf.Write(data)
	a.Equal(n, 5)
	a.Nil(err)
	data = make([]byte, 11)
	n, err = buf.Read(data)
	a.Equal(n, 11)
	a.Nil(err)
	a.Equal(string(data), "hello world")
	a.Equal(buf.Size(), int64(0))
	a.Equal(buf.Capacity(), int64(1024))
}

func TestWriteExpandCapapcity(t *testing.T) {
	a := assert.New(t)
	buf := newRingBuffer(8)
	data := []byte("hello ")
	buf.Write(data)
	data = []byte("world")
	n, err := buf.Write(data)
	a.Equal(n, 5)
	a.Nil(err)
	data = make([]byte, 11)
	n, err = buf.Read(data)
	a.Equal(n, 11)
	a.Nil(err)
	a.Equal(string(data), "hello world")
	a.Equal(buf.Size(), int64(0))
	a.Equal(buf.Capacity(), int64(11))
}

func TestWriteRound(t *testing.T) {
	a := assert.New(t)
	buf := newRingBuffer(8)
	data := []byte("hello ")
	buf.Write(data)
	buf.Read(data)
	a.Equal(buf.Size(), int64(0))
	data = []byte("world")
	n, err := buf.Write(data)
	a.Equal(n, 5)
	a.Nil(err)
	a.Equal(buf.Capacity(), int64(8))
	d := make([]byte, 5)
	buf.Read(d[:])
	a.Equal(string(d), "world")
}
