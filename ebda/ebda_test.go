package ebda_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/ebda"
)

func TestNew(t *testing.T) {
	t.Parallel()

	m, err := ebda.New(4)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if len(bytes) != 1388 {
		t.Fatalf("Invalid size: %v", len(bytes))
	}
}
