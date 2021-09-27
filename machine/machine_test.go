package machine_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/machine"
)

func TestNewAndLoadLinux(t *testing.T) {
	t.Parallel()

	m, err := machine.New(1)
	if err != nil {
		t.Fatal(err)
	}

	if err = m.LoadLinux("../bzImage", "../initrd", "console=ttyS0"); err != nil {
		t.Fatal(err)
	}

	m.GetInputChan()
	m.InjectSerialIRQ()
	m.RunData()

	for i := 0; i < 10; i++ {
		isContinue, err := m.RunOnce(0)
		if err != nil {
			t.Fatal(err)
		}

		if !isContinue {
			t.Fatal("guest finished unexpectedly")
		}
	}
}
