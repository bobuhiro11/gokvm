package term_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/term"
)

func TestIsTerminal(t *testing.T) {
	t.Parallel()

	if term.IsTerminal() {
		t.Fatalf("it is not terminal")
	}
}
