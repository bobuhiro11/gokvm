package bootproto_test

import (
	"testing"

	"github.com/nmi/gokvm/bootproto"
)

func TestNew(t *testing.T) {
	t.Parallel()

	if _, err := bootproto.New("../bzImage"); err != nil {
		t.Fatal(err)
	}
}

func TestBytes(t *testing.T) {
	t.Parallel()

	b, _ := bootproto.New("../bzImage")

	if _, err := b.Bytes(); err != nil {
		t.Fatal(err)
	}
}
