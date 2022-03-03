package tap_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/tap"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tap, err := tap.New("test_tap")
	if err != nil {
		t.Fatal(err)
	}

	err = tap.Close()
	if err != nil {
		t.Fatal(err)
	}
}
