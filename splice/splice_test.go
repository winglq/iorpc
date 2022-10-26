package splice

import (
	"os"
	"testing"
)

func TestPairSize(t *testing.T) {
	p, _ := Get()
	defer Done(p)

	p.MaxGrow()
	b := make([]byte, p.Cap()+100)
	for i := range b {
		b[i] = byte(i)
	}

	f, _ := os.CreateTemp("", "splice")
	err := os.WriteFile(f.Name(), b, 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = p.LoadFrom(f.Fd(), len(b), 0)
	if err == nil {
		t.Fatalf("should give error on exceeding capacity")
	}

}

func TestDiscard(t *testing.T) {
	p, _ := Get()
	defer Done(p)

	if _, err := p.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	p.discard()

	var b [1]byte
	n, err := p.Read(b[:])
	if n != -1 {
		t.Fatalf("Read: got (%d, %v) want (-1, EAGAIN)", n, err)
	}
}
