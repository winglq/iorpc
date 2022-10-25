package splice

import (
	"fmt"
	"log"
	"syscall"
)

func (p *Pair) LoadFromAt(fd uintptr, sz int, off int64) (int, error) {
	if sz > p.size {
		return 0, fmt.Errorf("LoadFrom: not enough space %d, %d",
			sz, p.size)
	}

	n, err := syscall.Splice(int(fd), &off, p.w, nil, sz, 0)
	return int(n), err
}

func (p *Pair) LoadFrom(fd uintptr, sz int) (int, error) {
	return p.LoadFromAt(fd, sz, 0)
}

func (p *Pair) WriteTo(fd uintptr, n int) (int, error) {
	m, err := syscall.Splice(p.r, nil, int(fd), nil, int(n), 0)
	return int(m), err
}

const _SPLICE_F_NONBLOCK = 0x2

func (p *Pair) discard() {
	_, err := syscall.Splice(p.r, nil, int(devNullFD), nil, int(p.size), _SPLICE_F_NONBLOCK)
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
