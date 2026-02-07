package serial_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/bobuhiro11/gokvm/serial"
)

type mockInjector struct{}

func (m *mockInjector) InjectSerialIRQ() error {
	return nil
}

func TestNew(t *testing.T) {
	t.Parallel()

	s, err := serial.New(&mockInjector{})
	s.GetInputChan()

	if err != nil {
		t.Fatal(err)
	}
}

func TestIn(t *testing.T) {
	t.Parallel()

	s, err := serial.New(&mockInjector{})
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

	s, err := serial.New(&mockInjector{})
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

func TestStartSerial(t *testing.T) {
	t.Parallel()

	s, err := serial.New(&mockInjector{})
	if err != nil {
		t.Fatal(err)
	}

	injectFunc := func() error {
		return nil
	}

	var bufIn bytes.Buffer

	if _, err := bufIn.Write([]byte{'T', 'E', 'S', 'T'}); err != nil {
		t.Fatal(err)
	}

	in := bufio.NewReader(&bufIn)

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := s.Start(*in, func() {}, injectFunc); !errors.Is(err, io.EOF) {
			t.Errorf("s.Start(): got %v, want %v", err, io.EOF)
		}
	}()

	if err := s.In(serial.COM1Addr+3, []byte{0}); err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}
