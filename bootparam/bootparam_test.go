package bootparam_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/nmi/gokvm/bootparam"
)

func TestNew(t *testing.T) {
	t.Parallel()

	if _, err := bootparam.New("../bzImage"); err != nil {
		t.Fatal(err)
	}
}

func TestNewNotbzImage(t *testing.T) {
	t.Parallel()

	if _, err := bootparam.New("../README.md"); err == nil {
		t.Fatal(err)
	}
}

func TestBytes(t *testing.T) {
	t.Parallel()

	b, _ := bootparam.New("../bzImage")

	if _, err := b.Bytes(); err != nil {
		t.Fatal(err)
	}
}

func TestAddE820Entry(t *testing.T) {
	t.Parallel()

	b, _ := bootparam.New("../bzImage")
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
