package pci_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/bobuhiro11/gokvm/pci"
)

func TestSizeToBits(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		input    uint64
		expected uint32
	}{
		{
			name:     "Success",
			input:    0x100,
			expected: 0xffffff00,
		},
		{
			name:     "Fail",
			input:    0x0,
			expected: 0x0,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.expected != pci.SizeToBits(tt.input) {
				t.Fatalf("expected: %v, actual: %v", tt.expected, tt.input)
			}
		})
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
	_ = p.PciConfAddrIn(0xCF8, pci.NumToBytes(uint32(0x80000010)))  // random call to PciConfAddrIn

	bytes := make([]byte, 4)
	_ = p.PciConfDataIn(0xCFC, bytes)
	actual := uint32(pci.BytesToNum(bytes))

	if expected != actual {
		t.Fatalf("expected: 0x%x, actual: 0x%x", expected, actual)
	}
}

func TestBytes(t *testing.T) {
	t.Parallel()

	dh := pci.DeviceHeader{
		DeviceID:      1,
		VendorID:      1,
		HeaderType:    1,
		SubsystemID:   1,
		Command:       1,
		BAR:           [6]uint32{},
		InterruptPin:  1,
		InterruptLine: 1,
	}

	b, err := dh.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if b[0] != byte(dh.VendorID) {
		t.Fatalf("invalid vendor id")
	}
}

func TestPciConfAddrInOut(t *testing.T) {
	t.Parallel()

	p := pci.New(pci.NewBridge())

	for _, tt := range []struct {
		name string
		port uint64
		data []byte
		exp  error
	}{
		{
			name: "Success",
			port: 0x0,
			data: make([]byte, 4),
			exp:  nil,
		},
		{
			name: "Fail_DataLength",
			port: 0x0,
			data: make([]byte, 3),
			exp:  nil,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := p.PciConfAddrIn(tt.port, tt.data); !errors.Is(err, tt.exp) {
				t.Fatalf("%s failed: %v", tt.name, err)
			}

			if err := p.PciConfAddrOut(tt.port, tt.data); !errors.Is(err, tt.exp) {
				t.Fatalf("%s failed: %v", tt.name, err)
			}
		})
	}
}
