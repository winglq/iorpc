package iorpc

import (
	"io"
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
	File() (fd uintptr)
}

type IsConn interface {
	io.Closer
	syscall.Conn
}

type IsPipe interface {
	io.ReadCloser
	ReadFd() (fd uintptr)
	WriteTo(fd uintptr, n int, flags int) (int, error)
}

type IsBuffer interface {
	io.Closer
	Iovec() [][]byte
}

type noopBody struct{}

func (noopBody) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (noopBody) Close() error {
	return nil
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
	err = pair.Grow(alignSize(size))
	if err != nil {
		return nil, errors.Wrap(err, "grow pipe pair")
	}
	_, err = pair.LoadFromAt(r.File(), size, &offset, splice.SPLICE_F_MOVE)
	if err != nil {
		return nil, errors.Wrap(err, "pair load file")
	}
	return &Pipe{pair: pair}, nil
}

func alignSize(size int) int {
	pageSize := os.Getpagesize()
	return size + pageSize - size%pageSize
}

func PipeConn(r IsConn, size int) (IsPipe, error) {
	pair, err := splice.Get()
	if err != nil {
		return nil, errors.Wrap(err, "get pipe pair")
	}
	err = pair.Grow(alignSize(size))
	if err != nil {
		return nil, errors.Wrap(err, "grow pipe pair")
	}

	rawConn, err := r.SyscallConn()
	if err != nil {
		return nil, errors.Wrap(err, "get raw conn")
	}

	loaded := 0
	var loadError error

	err = rawConn.Read(func(fd uintptr) (done bool) {
		var n int
		n, loadError = pair.LoadFrom(fd, size-loaded, splice.SPLICE_F_NONBLOCK|splice.SPLICE_F_MOVE)
		if loadError != nil {
			return loadError != syscall.EAGAIN && loadError != syscall.EINTR
		}
		loaded += n
		return loaded == size
	})

	if err == nil {
		err = loadError
	}
	if err != nil {
		return nil, errors.Wrap(err, "pair load file")
	}
	return &Pipe{pair: pair}, nil
}

func PipeBuffer(r IsBuffer, size int) (IsPipe, error) {
	pair, err := splice.Get()
	if err != nil {
		return nil, errors.Wrap(err, "get pipe pair")
	}
	err = pair.Grow(alignSize(size))
	if err != nil {
		return nil, errors.Wrap(err, "grow pipe pair")
	}

	for _, slice := range r.Iovec() {
		// TODO: use vmsplice to load buffer
		// There is a bug in vmsplice, it will cause data corruption
		// _, err = pair.LoadBuffer(r.Iovec(), size, splice.SPLICE_F_GIFT)
		_, err = syscall.Write(int(pair.WriteFd()), slice)
		if err != nil {
			return nil, errors.Wrap(err, "pair load buffer")
		}
	}

	return &Pipe{pair: pair}, nil
}

func (p *Pipe) ReadFd() uintptr {
	return p.pair.ReadFd()
}

func (p *Pipe) WriteTo(fd uintptr, n int, flags int) (int, error) {
	if p.pair == nil {
		return 0, io.EOF
	}
	return p.pair.WriteTo(fd, n, flags)
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
	syscallConn, ok := w.(syscall.Conn)
	if !ok {
		return false, nil
	}

	dstRawConn, err := syscallConn.SyscallConn()
	if err != nil {
		return false, nil
	}

	flags := splice.SPLICE_F_NONBLOCK | splice.SPLICE_F_MOVE
	var pipe IsPipe
	switch reader := b.Reader.(type) {
	case IsPipe:
		pipe = reader
	case IsFile:
		pipe, err = PipeFile(reader, int64(b.Offset), int(b.Size))
	case IsConn:
		pipe, err = PipeConn(reader, int(b.Size))
	case IsBuffer:
		pipe, err = PipeBuffer(reader, int(b.Size))
	default:
		return false, nil
	}

	if err != nil {
		// fail to load reader, fallback to normal copy
		return false, nil
	}
	defer pipe.Close()

	written := uint64(0)
	var writeError error
	err = dstRawConn.Write(func(fd uintptr) (done bool) {
		var n int
		n, writeError = pipe.WriteTo(fd, int(b.Size-written), flags)
		if writeError != nil {
			return writeError != syscall.EAGAIN && writeError != syscall.EINTR
		}
		written += uint64(n)
		return written == b.Size
	})
	if err == nil {
		err = writeError
	}
	return true, err
}
