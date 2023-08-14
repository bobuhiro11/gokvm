package term_test

import (
	"errors"
	"testing"

	"github.com/bobuhiro11/gokvm/term"
	"golang.org/x/sys/unix"
)

func TestIsTerminal(t *testing.T) {
	t.Parallel()

	if term.IsTerminal() {
		t.Fatalf("it is not terminal")
	}
}

func TestSetRawMode(t *testing.T) {
	t.Parallel()

	if _, err := term.SetRawMode(); err != nil && !errors.Is(err, unix.ENOTTY) {
		t.Fatalf("error SetRawMode: %v", err)
	}
}
