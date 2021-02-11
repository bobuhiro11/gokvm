package serial_test

import (
	"testing"

	"github.com/nmi/gokvm/serial"
)

func TestNew(t *testing.T) {
	t.Parallel()

	callback := func(irq, level uint32) {}
	s, err := serial.New(callback)
	s.GetInputChan()
	s.InjectIRQ()

	if err != nil {
		t.Fatal(err)
	}
}

func TestIn(t *testing.T) {
	t.Parallel()

	callback := func(irq, level uint32) {}

	s, err := serial.New(callback)
	if err != nil {
		t.Fatal(err)
	}

	// Here the unit test call the function simply.
	// It needs to be fixed.
	for i := 0; i < 8; i++ {
		if err := s.In(uint64(serial.COM1Addr+i), []byte{0}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOut(t *testing.T) {
	t.Parallel()

	callback := func(irq, level uint32) {}

	s, err := serial.New(callback)
	if err != nil {
		t.Fatal(err)
	}

	// Here the unit test call the function simply.
	// It needs to be fixed.
	for i := 0; i < 8; i++ {
		if err := s.Out(uint64(serial.COM1Addr+i), []byte{0}); err != nil {
			t.Fatal(err)
		}
	}
}
