package iorpc

import (
	"bytes"
	"io"
	"net"
	"os"
	"syscall"
)

type Body struct {
	Offset, Size uint64
	Reader       io.ReadCloser
	NotClose     bool
}

type IsFile interface {
	io.Closer
	File() *os.File
}

type IsConn interface {
	io.Closer
	Conn() net.Conn
	SyscallConn() (c syscall.Conn, err error)
}

type IsPipe interface {
	io.Closer
	Pipe() (fd int)
}

type IsBuffer interface {
	io.Closer
	Buffer() *bytes.Buffer
}

func (b *Body) Reset() {
	b.Offset, b.Size, b.Reader, b.NotClose = 0, 0, nil, false
}

func (b *Body) Close() error {
	if b.NotClose || b.Reader == nil {
		return nil
	}
	return b.Reader.Close()
}
