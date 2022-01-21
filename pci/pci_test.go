package pci_test

import (
	"bytes"
	"testing"

	"github.com/bobuhiro11/gokvm/pci"
)

func TestSizeToBits(t *testing.T) {
	t.Parallel()

	expected := uint32(0xffffff00)
	actual := pci.SizeToBits(0x100)

	if expected != actual {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestBytesToNum(t *testing.T) {
	t.Parallel()

	expected := uint64(0x12345678)
	actual := pci.BytesToNum([]byte{0x78, 0x56, 0x34, 0x12})

	if expected != actual {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNumToBytes8(t *testing.T) {
	t.Parallel()

	expected := []byte{0x12}
	actual := pci.NumToBytes(uint8(0x12))

	if !bytes.Equal(actual, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNumToBytes16(t *testing.T) {
	t.Parallel()

	expected := []byte{0x34, 0x12}
	actual := pci.NumToBytes(uint16(0x1234))

	if !bytes.Equal(actual, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNumToBytes32(t *testing.T) {
	t.Parallel()

	expected := []byte{0x78, 0x56, 0x34, 0x12}
	actual := pci.NumToBytes(uint32(0x12345678))

	if !bytes.Equal(actual, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNumToBytes64(t *testing.T) {
	t.Parallel()

	expected := []byte{0x78, 0x56, 0x34, 0x12, 0x78, 0x56, 0x34, 0x12}
	actual := pci.NumToBytes(uint64(0x1234567812345678))

	if !bytes.Equal(actual, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNumToBytesInvalid(t *testing.T) {
	t.Parallel()

	actual := pci.NumToBytes(-1)
	expected := []byte{}

	if !bytes.Equal(actual, expected) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestProbingBAR0(t *testing.T) {
	t.Parallel()

	br := pci.NewBridge()
	start, end := br.GetIORange()
	expected := pci.SizeToBits(end - start)

	p := pci.New(br)
	_ = p.PciConfAddrOut(0x0, pci.NumToBytes(uint32(0x80000010)))   // offset 0x10 for BAR0 with enable bit 0x80
	_ = p.PciConfDataOut(0xCFC, pci.NumToBytes(uint32(0xffffffff))) // all 1-bits for probing size of BAR0

	bytes := make([]byte, 4)
	_ = p.PciConfDataIn(0xCFC, bytes)
	actual := uint32(pci.BytesToNum(bytes))

	if expected != actual {
		t.Fatalf("expected: 0x%x, actual: 0x%x", expected, actual)
	}
}
