package bootparam_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/bobuhiro11/gokvm/bootparam"
)

func bpnew(n string) (*bootparam.BootParam, error) {
	f, err := os.Open(n)
	if err != nil {
		return nil, fmt.Errorf("Skipping this test: %w", err)
	}

	return bootparam.New(f)
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Do a test open for the bzimage. If it fails for any reason,
	// just skip this test.
	if _, err := bpnew("../bzImage"); err != nil {
		t.Skipf("Skipping this test: %v", err)
	}
}

func TestNewNotbzImage(t *testing.T) {
	t.Parallel()

	if _, err := bpnew("../README.md"); err == nil {
		t.Fatal(err)
	}
}

func TestBytes(t *testing.T) {
	t.Parallel()

	b, err := bpnew("../bzImage")
	if err != nil {
		t.Skipf("Skipping this test: %v", err)
	}

	if _, err := b.Bytes(); err != nil {
		t.Fatal(err)
	}
}

func TestAddE820Entry(t *testing.T) {
	t.Parallel()

	b, err := bpnew("../bzImage")
	if err != nil {
		t.Skipf("Skipping this test: %v", err)
	}

	b.AddE820Entry(
		0x1234567812345678,
		0xabcdefabcdefabcd,
		bootparam.E820Ram,
	)

	rawBootParam, _ := b.Bytes()
	if rawBootParam[0x1E8] != 1 {
		t.Fatalf("invalid e820_entries: %d", rawBootParam[0x1E8])
	}

	actual := bootparam.E820Entry{}
	reader := bytes.NewReader(rawBootParam[0x2D0:])

	if err := binary.Read(reader, binary.LittleEndian, &actual); err != nil {
		t.Fatal(err)
	}

	if actual.Addr != 0x1234567812345678 {
		t.Fatalf("invalid e820 addr: %v", actual.Addr)
	}

	if actual.Size != 0xabcdefabcdefabcd {
		t.Fatalf("invalid e820 size: %v", actual.Size)
	}

	if actual.Type != bootparam.E820Ram {
		t.Fatalf("invalid e820 type: %v", actual.Type)
	}
}
