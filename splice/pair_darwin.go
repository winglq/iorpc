package splice

func (p *Pair) LoadFromAt(fd uintptr, sz int, off *int64, flags int) (int, error) {
	panic("not implemented")
	return 0, nil
}

func (p *Pair) LoadFrom(fd uintptr, sz int, flags int) (int, error) {
	panic("not implemented")
	return 0, nil
}

func (p *Pair) WriteTo(fd uintptr, n int, flags int) (int, error) {
	panic("not implemented")
	return 0, nil
}

func (p *Pair) discard() {
	panic("not implemented")
}
