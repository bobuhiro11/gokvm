package machine_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/machine"
)

func TestNewAndLoadLinux(t *testing.T) { // nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	m, err := machine.New("/dev/kvm", 1, "tap", "../vda.img", 1<<30)
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

	initrd, err := os.Open("../initrd")
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

	time.Sleep(15 * time.Second)

	output, err := exec.Command("ping", "192.168.20.1", "-c", "3", "-i", "0.1").Output()
	t.Logf("ping output: %s\n", output)

	if err != nil {
		t.Fatal(err)
	}

	output, err = exec.Command("curl", "--retry", "5", "--retry-delay", "3", "-L",
		"192.168.20.1/mnt/dev_vda/index.html").Output()
	t.Logf("curl output: %s\n", output)

	if err != nil {
		t.Fatal(err)
	}

	if string(output) != "index.html: this message is from /dev/vda in guest\n" {
		t.Fatal(string(output))
	}
}

// TestHalt tries to run a Halt instruction in 64-bit mode.
func TestHalt(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, "", "", 1<<30)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SetupRegs(0x1_00_000, 0x10_000, true); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	r, err := m.GetRegs(0)
	if err != nil {
		t.Errorf("GetRegs before RunOnce: %v != nil", err)
	}

	if r.RAX != 0 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0)
	}

	if r.RIP != 0x1_00_000 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RIP, 0x1_00_000)
	}

	t.Logf("Registers %#x", r)

	ok, err := m.RunOnce(0)
	if err == nil {
		t.Errorf("Run: got %v, %v, want nil", ok, err)
	}

	t.Logf("RunOnce: %v,%v", ok, err)

	if !errors.Is(err, kvm.ErrUnexpectedEXITReason) {
		t.Errorf("Run: RunOnce(0) exit is %v, not %v", err, kvm.ErrUnexpectedEXITReason)
	}

	if s, err := m.GetSRegs(0); err != nil {
		t.Errorf("Run: GetRegs(0) exit is %v, not nil", err)
	} else {
		t.Logf("GetSRegs: %#x", s)
	}

	if r, err = m.GetRegs(0); err != nil {
		t.Errorf("Run: GetRegs(0) exit is %v, not nil", err)
	}

	if r.RAX != 0xcafebabe {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0xcafebabe)
	}

	if r.RIP != 0x1_00_006 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RIP, 0x1_00_006)
	}

	for _, addr := range []uint64{0, 0x100_000, 0x400_000, 0x1_000_000, 0x1_0000_0000, 0x10_0000_0000} {
		trs, err := m.Translate(addr)
		if err != nil {
			t.Errorf("Translate(%#x): %v != nil", addr, err)

			continue
		}

		for i, tr := range trs {
			// another golang-ci antipattern
			t.Logf("Translate(%d, %#x): pa %#x, Valid %#x, Writeable %#x, Usermode %#x",
				i, addr, tr.PhysicalAddress, tr.Valid, tr.Writeable, tr.Usermode)
		}
	}
}

func TestReadAt(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, "", "", 1<<30)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	var (
		b   [4]byte
		off int64 = 0x1_000_000
	)

	if n, err := m.ReadAt(b[:], off); err != nil || n != len(b) {
		t.Fatalf("ReedAt(b, %#x): (%d,%v) != (%d,nil)", off, n, err, len(b))
	}

	if !bytes.Equal(b[:], []byte(machine.Poison)[:4]) {
		t.Fatalf("ReadAt(b, %#x): %#x != %#x", off, b, machine.Poison)
	}

	t.Logf("%#x found at %#x", b, off)
}

func TestSingleStepOffOn(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, "", "", 1<<30)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SingleStep(false); err != nil {
		t.Errorf("SingleStep(false): got %v, want nil", err)
	}

	if err := m.SingleStep(true); err != nil {
		t.Fatalf("SingleStep(true): got %v, want nil", err)
	}

	if err := m.SingleStep(false); err != nil {
		t.Errorf("SingleStep(false): got %v, want nil", err)
	}
}

// TestHalt tries to run a Halt instruction in 64-bit mode.
func TestSetupGetSetRegs(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, "", "", 1<<30)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SetupRegs(0x1_000_000, 0x10_000, false); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	r, err := m.GetRegs(0)
	if err != nil {
		t.Fatalf("GetRegs: got %v, want nil", err)
	}

	t.Logf("Regs %#x r.RIP %#x", r, r.RIP)

	rip := r.RIP + 1
	r.RIP = rip

	var core int
	if err := m.SetRegs(core, r); err != nil {
		t.Fatalf("setregs: got %v, want nil", err)
	}

	r, err = m.GetRegs(0)
	if err != nil {
		t.Fatalf("GetRegs: got %v, want nil", err)
	}

	if r.RIP != rip {
		t.Fatalf("TestRegs: r.RIP is %#x, want %#x", r.RIP, rip)
	}

	t.Logf("r.RIP %#x", r.RIP)
}

func TestSingleStep(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, "", "", 1<<30)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SetupRegs(0x1_00_000, 0x10_000, true); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	r, err := m.GetRegs(0)
	if err != nil {
		t.Errorf("GetRegs before RunOnce: %v != nil", err)
	}

	if r.RAX != 0 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0)
	}

	if r.RIP != 0x1_00_000 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RIP, 0x1_00_000)
	}

	ok, err := m.RunOnce(0)
	if err == nil {
		t.Errorf("Run: got %v, %v, want nil", ok, err)
	}

	t.Logf("Runonce: %v, %v", ok, err)

	if !errors.Is(err, kvm.ErrUnexpectedEXITReason) {
		t.Errorf("Run: RunOnce(0) exit is %v, not %v", err, kvm.ErrUnexpectedEXITReason)
	}

	if r, err = m.GetRegs(0); err != nil {
		t.Errorf("Run: GetRegs(0) exit is %v, not nil", err)
	}

	if r.RAX != 0xcafebabe {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0xcafebabe)
	}

	if r.RIP != 0x1_00_006 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RIP, 0x1_00_006)
	}
}

func TestTranslate32(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, "", "", 1<<30)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SetupRegs(0x1_00_000, 0x10_000, false); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	r, err := m.GetRegs(0)
	if err != nil {
		t.Errorf("GetRegs before RunOnce: %v != nil", err)
	}

	if r.RAX != 0 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0)
	}

	if r.RIP != 0x1_00_000 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RIP, 0x1_00_000)
	}

	ok, err := m.RunOnce(0)
	if err == nil {
		t.Errorf("Run: got %v, %v, want nil", ok, err)
	}

	t.Logf("Runonce: %v, %v", ok, err)

	if !errors.Is(err, kvm.ErrUnexpectedEXITReason) {
		t.Errorf("Run: RunOnce(0) exit is %v, not %v", err, kvm.ErrUnexpectedEXITReason)
	}

	if r, err = m.GetRegs(0); err != nil {
		t.Errorf("Run: GetRegs(0) exit is %v, not nil", err)
	}

	if r.RAX != 0xcafebabe {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0xcafebabe)
	}

	if r.RIP != 0x1_00_006 {
		t.Errorf("Run: RAX is %#x, not %#x", r.RIP, 0x1_00_006)
	}

	trs, err := m.Translate(0x100000)
	if err != nil {
		t.Errorf("Translate(0x100000): %v != nil", err)
	}

	for i, tr := range trs {
		t.Logf("Translate(%d, 0x100000): pa %#x, Valid %#x, Writeable %#x, Usermode %#x",
			i, tr.PhysicalAddress, tr.Valid, tr.Writeable, tr.Usermode)
	}
}
