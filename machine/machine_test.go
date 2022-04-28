package machine_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/bobuhiro11/gokvm/machine"
)

func TestNewAndLoadLinux(t *testing.T) { // nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	m, err := machine.New(1, "tap", "../vda.img")
	if err != nil {
		t.Fatal(err)
	}

	param := `console=ttyS0 earlyprintk=serial noapic noacpi notsc ` +
		`lapic tsc_early_khz=2000 pci=realloc=off virtio_pci.force_legacy=1 ` +
		`rdinit=/init init=/init`

	kern, err := os.Open("../bzImage")
	if err != nil {
		t.Fatal(err)
	}

	initrd, err := os.Open("../goinitrd")
	if err != nil {
		t.Fatal(err)
	}

	if err = m.LoadLinux(kern, initrd, param); err != nil {
		t.Fatal(err)
	}

	m.GetInputChan()

	if err := m.InjectSerialIRQ(); err != nil {
		t.Errorf("m.InjectSerialIRQ: %v != nil", err)
	}

	m.RunData()

	go func() {
		if err = m.RunInfiniteLoop(0); err != nil {
			panic(err)
		}
	}()

	if err := exec.Command("ip", "link", "set", "tap", "up").Run(); err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("ip", "addr", "add", "192.168.20.2/24", "dev", "tap").Run(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Second)

	output, err := exec.Command("ping", "192.168.20.1", "-c", "3", "-i", "0.1").Output()
	t.Logf("ping output: %s\n", output)

	if err != nil {
		t.Fatal(err)
	}

	output, err = exec.Command("curl", "--retry", "5", "-L", "192.168.20.1/mnt/dev_vda/index.html").Output()
	t.Logf("curl output: %s\n", output)

	if err != nil {
		t.Fatal(err)
	}

	if string(output) != "index.html: this message is from /dev/vda in guest\n" {
		t.Fatal(string(output))
	}
}
