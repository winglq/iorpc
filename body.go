package iorpc

import (
	"bytes"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/hexilee/iorpc/splice"
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

func (b *Body) spliceTo(w io.Writer) (bool, error) {
	file, ok := b.Reader.(IsFile)
	if !ok {
		return false, nil
	}

	syscallConn, ok := w.(syscall.Conn)
	if !ok {
		return false, nil
	}

	dstRawConn, err := syscallConn.SyscallConn()
	if err != nil {
		return false, nil
	}
	pair, err := splice.Get()
	if err != nil {
		return false, nil
	}
	pair.Grow(int(b.Size))
	defer splice.Done(pair)
	_, err = pair.LoadFromAt(file.File().Fd(), int(b.Size), int64(b.Offset))
	if err != nil {
		return false, nil
	}

	written := uint64(0)
	var writeError error
	err = dstRawConn.Write(func(fd uintptr) (done bool) {
		var n int
		n, writeError = pair.WriteTo(fd, int(b.Size-written))
		if writeError == syscall.EAGAIN || writeError == syscall.EINTR {
			writeError = nil
			return false
		}
		if writeError != nil {
			return true
		}
		written += uint64(n)
		return written == b.Size
	})
	if err == nil {
		err = writeError
	}
	return true, err
}
