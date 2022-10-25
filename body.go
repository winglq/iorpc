package iorpc

import (
	"bytes"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/hexilee/iorpc/splice"
	"github.com/pkg/errors"
)

type Body struct {
	Offset, Size uint64
	Reader       io.ReadCloser
	NotClose     bool
}

type Pipe struct {
	pair *splice.Pair
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
	WriteTo(fd uintptr, n int) (int, error)
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

func PipeFile(r IsFile, offset int64, size int) (IsPipe, error) {
	pair, err := splice.Get()
	if err != nil {
		return nil, errors.Wrap(err, "get pipe pair")
	}
	err = pair.Grow(size)
	if err != nil {
		return nil, errors.Wrap(err, "grow pipe pair")
	}
	_, err = pair.LoadFromAt(r.File().Fd(), size, offset)
	if err != nil {
		return nil, errors.Wrap(err, "pair load file")
	}
	return &Pipe{pair: pair}, nil
}

func PipeConn(r IsConn, size int) (IsPipe, error) {
	return nil, errors.New("pipe conn is unsupported")
}

func PipeBuffer(r IsBuffer, offset int64, size int) (IsPipe, error) {
	return nil, errors.New("pipe buffer is unsupported")
}

func (p *Pipe) WriteTo(fd uintptr, n int) (int, error) {
	if p.pair == nil {
		return 0, io.EOF
	}
	return p.pair.WriteTo(fd, n)
}

func (p *Pipe) Read(b []byte) (int, error) {
	if p.pair == nil {
		return 0, io.EOF
	}
	return p.pair.Read(b)
}

func (p *Pipe) Close() error {
	if p.pair == nil {
		return nil
	}
	splice.Done(p.pair)
	return nil
}

func (b *Body) spliceTo(w io.Writer) (bool, error) {
	var pipe IsPipe
	var err error
	switch reader := b.Reader.(type) {
	case IsPipe:
		pipe = reader
	case IsFile:
		pipe, err = PipeFile(reader, int64(b.Offset), int(b.Size))
		if err != nil {
			return false, errors.Wrap(err, "pipe file")
		}
		defer pipe.Close()
	case IsConn:
		pipe, err = PipeConn(reader, int(b.Size))
		if err != nil {
			return false, errors.Wrap(err, "pipe conn")
		}
		defer pipe.Close()
	case IsBuffer:
		pipe, err = PipeBuffer(reader, int64(b.Offset), int(b.Size))
		if err != nil {
			return false, errors.Wrap(err, "pipe buffer")
		}
		defer pipe.Close()
	default:
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

	written := uint64(0)
	var writeError error
	err = dstRawConn.Write(func(fd uintptr) (done bool) {
		var n int
		n, writeError = pipe.WriteTo(fd, int(b.Size-written))
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
