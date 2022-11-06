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
