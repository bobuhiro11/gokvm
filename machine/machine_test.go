package machine_test

import (
	"testing"

	"github.com/nmi/gokvm/machine"
)

func TestNewAndLoadLinux(t *testing.T) {
	t.Parallel()

	m, err := machine.New()
	if err != nil {
		t.Fatal(err)
	}

	if err = m.LoadLinux("../bzImage", "../initrd"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		isContinue, err := m.RunOnce()
		if err != nil {
			t.Fatal(err)
		}

		if !isContinue {
			t.Fatal("guest finished unexpectedly")
		}
	}
}
