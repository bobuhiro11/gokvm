package pci_test

import (
	"errors"
	"testing"

	"github.com/bobuhiro11/gokvm/pci"
)

func TestGetDeviceHeader(t *testing.T) {
	t.Parallel()

	br := pci.NewBridge()
	expected := uint16(0x0d57)
	actual := br.GetDeviceHeader().DeviceID

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestIOHanders(t *testing.T) {
	t.Parallel()

	expected := pci.ErrIONotPermit
	br := pci.NewBridge()

	if actual := br.Read(0x0, []byte{}); !errors.Is(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}

	if actual := br.Write(0x0, []byte{}); !errors.Is(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestGetIORange(t *testing.T) {
	t.Parallel()

	expected := uint64(0x10)
	actual := pci.NewBridge().Size()

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}
