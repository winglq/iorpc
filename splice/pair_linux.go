package splice

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

func vmsplice(fd int, iovs []syscall.Iovec, flags int) (int, error) {
	n, _, errno := syscall.Syscall6(
		syscall.SYS_VMSPLICE,
		uintptr(fd),
		uintptr(unsafe.Pointer(&iovs[0])),
		uintptr(len(iovs)),
		uintptr(flags),
		0,
		0,
	)
	if errno != 0 {
		return 0, errno
	}

	return int(n), nil
}

func (p *Pair) LoadFromAt(fd uintptr, sz int, off *int64, flags int) (int, error) {
	if sz > p.size {
		return 0, fmt.Errorf("LoadFrom: not enough space %d, %d",
			sz, p.size)
	}

	n, err := syscall.Splice(int(fd), off, p.w, nil, sz, flags)
	return int(n), err
}

func (p *Pair) LoadFrom(fd uintptr, sz int, flags int) (int, error) {
	return p.LoadFromAt(fd, sz, nil, flags)
}

func (p *Pair) LoadBuffer(buffer [][]byte, sz int, flags int) (int, error) {
	if sz > p.size {
		return 0, fmt.Errorf("LoadFrom: not enough space %d, %d",
			sz, p.size)
	}
	iovec := make([]syscall.Iovec, 0, len(buffer))
	for _, b := range buffer {
		iovec = append(iovec, syscall.Iovec{Base: &b[0], Len: uint64(len(b))})
	}
	return vmsplice(p.w, iovec, flags)
}

func (p *Pair) WriteTo(fd uintptr, n int, flags int) (int, error) {
	m, err := syscall.Splice(p.r, nil, int(fd), nil, int(n), flags)
	return int(m), err
}

func (p *Pair) discard() {
	_, err := syscall.Splice(p.r, nil, int(devNullFD), nil, int(p.size), SPLICE_F_NONBLOCK)
	if err == syscall.EAGAIN {
		// all good.
	} else if err != nil {
		errR := syscall.Close(p.r)
		errW := syscall.Close(p.w)

		// This can happen if something closed our fd
		// inadvertently (eg. double close)
		log.Panicf("splicing into /dev/null: %v (close R %d '%v', close W %d '%v')", err, p.r, errR, p.w, errW)
	}
}
