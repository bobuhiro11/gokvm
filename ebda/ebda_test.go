package ebda_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/ebda"
)

func TestNewMPFIntel(t *testing.T) {
	t.Parallel()

	m, err := ebda.NewMPFIntel()
	if err != nil {
		t.Fatal(err)
	}

	checkSum, err := m.CalcCheckSum()
	if err != nil {
		t.Fatal(err)
	}

	if checkSum != 0 {
		t.Fatal("Invalid checkSum")
	}

	bytes, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if len(bytes) != 16 {
		t.Fatal("Invalid size")
	}
}
