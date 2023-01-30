package machine_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/machine"
	"golang.org/x/arch/x86/x86asm"
)

func testNewAndLoadLinux(t *testing.T, kernel, tap, guestIPv4, hostIPv4, prefixLen string) { // nolint:thelper
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	m, err := machine.New("/dev/kvm", 1, tap, "../vda.img", 1<<29)
	if err != nil {
		t.Fatal(err)
	}

	param := fmt.Sprintf(`console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
		`lapic tsc_early_khz=2000 pci=realloc=off virtio_pci.force_legacy=1 `+
		`rdinit=/init init=/init gokvm.ipv4_addr=%s/%s`, guestIPv4, prefixLen)

	kern, err := os.Open(kernel)
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

	if err := exec.Command("ip", "link", "set", tap, "up").Run(); err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("ip", "addr", "add", hostIPv4+"/"+prefixLen, "dev", tap).Run(); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	time.Sleep(15 * time.Second)

	output, err := exec.Command("ping", guestIPv4, "-c", "3", "-i", "0.1").Output()
	t.Logf("ping output: %s\n", output)

	if err != nil {
		t.Fatal(err)
	}

	output, err = exec.Command("curl", "--retry", "5", "--retry-delay", "3", "-L", //nolint:gosec
		fmt.Sprintf("%s/mnt/dev_vda/index.html", guestIPv4)).Output()
	t.Logf("curl output: %s\n", output)

	if err != nil {
		t.Fatal(err)
	}

	if string(output) != "index.html: this message is from /dev/vda in guest\n" {
		t.Fatal(string(output))
	}
}

func TestNewAndLoadLinuxWithBzImage(t *testing.T) { // nolint:paralleltest
	testNewAndLoadLinux(t, "../bzImage", "tap0", "192.168.20.1", "192.168.20.2", "24")
}

func TestNewAndLoadLinuxWithVmlinux(t *testing.T) { // nolint:paralleltest
	testNewAndLoadLinux(t, "../vmlinux", "tap1", "192.168.30.1", "192.168.30.2", "24")
}

// TestHalt tries to run a Halt instruction in 64-bit mode.
func TestHalt(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
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

	if !errors.Is(err, kvm.ErrUnexpectedExitReason) {
		t.Errorf("Run: RunOnce(0) exit is %v, not %v", err, kvm.ErrUnexpectedExitReason)
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

func TestReadWriteAt(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
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

	var zeros [8]byte
	if n, err := m.WriteAt(zeros[:], off); err != nil || n != len(zeros) {
		t.Fatalf("WriteAt(%#x, %#x): (%d, %v) != (%d, nil)", zeros, off, n, err, len(zeros))
	}

	if n, err := m.WriteAt(zeros[:], 1<<30); !errors.Is(err, syscall.EFBIG) {
		t.Fatalf("WriteAt(_, 1<<30): (%d, %v) != (%d, %v)", n, err, 0, syscall.EFBIG)
	}

	var got [8]byte
	if n, err := m.ReadAt(got[:], off); err != nil || n != len(got) {
		t.Fatalf("ReedAt(got, %#x): (%d,%v) != (%d,nil)", off, n, err, len(got))
	}

	if !bytes.Equal(got[:], zeros[:]) {
		t.Fatalf("ReadAt(b, %#x): %#x != %#x", off, got, zeros)
	}
}

func TestSingleStepOffOn(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
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

func TestSetupGetSetRegs(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SetupRegs(0x1_000_000, 0x10_000, false); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	if _, err := m.GetRegs(1024); err == nil {
		t.Errorf("GetRegs(1024): got nil, want err")
	}

	if _, err := m.GetSRegs(1024); err == nil {
		t.Errorf("GetSRegs(1024): got nil, want err")
	}

	r, err := m.GetRegs(0)
	if err != nil {
		t.Fatalf("GetRegs: got %v, want nil", err)
	}

	if err := m.SetRegs(1024, nil); err == nil {
		t.Errorf("SetRegs(1024, ...): got nil, want err")
	}

	if err := m.SetSRegs(1024, nil); err == nil {
		t.Errorf("SetSRegs(1024): got nil, want err")
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

func TestSingleStep(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if err := m.SetupRegs(0x1_00_000, 0x10_000, true); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	if err := m.SingleStep(true); err != nil {
		t.Fatalf("SingleStep(true): got %v, want nil", err)
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

	t.Logf("Before RunOnce, flags are %#x", r.RFLAGS)

	ok, err := m.RunOnce(0)
	if err == nil {
		t.Errorf("Run: got %v, %v, want nil", ok, err)
	}

	t.Logf("Runonce: %v, %v", ok, err)

	if !errors.Is(err, kvm.ErrDebug) {
		t.Errorf("Run: RunOnce(0) exit is %v, not %v", err, kvm.ErrDebug)
	}

	if r, err = m.GetRegs(0); err != nil {
		t.Errorf("Run: GetRegs(0) exit is %v, not nil", err)
	}

	if r.RAX != 0xcafebabe {
		t.Errorf("Run: RAX is %#x, not %#x", r.RAX, 0xcafebabe)
	}

	if r.RIP != 0x1_00_005 {
		t.Errorf("Run: RIP is %#x, not %#x", r.RIP, 0x1_00_005)
	}

	t.Logf("After RunOnce, flags are %#x", r.RFLAGS)

	if err := m.SingleStep(true); err != nil {
		t.Fatalf("SingleStep(true): got %v, want nil", err)
	}

	// It stepped once. See if it will step the NOP.
	ok, err = m.RunOnce(0)
	if err == nil {
		t.Errorf("Run: got %v, %v, want nil", ok, err)
	}

	t.Logf("Runonce: %v, %v", ok, err)

	if r, err = m.GetRegs(0); err != nil {
		t.Errorf("Run: GetRegs(0) exit is %v, not nil", err)
	}

	if r.RIP != 0x1_00_006 {
		t.Errorf("Run: RIP is %#x, not %#x", r.RIP, 0x1_00_006)
	}
}

func TestTranslate32(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
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

	if !errors.Is(err, kvm.ErrUnexpectedExitReason) {
		t.Errorf("Run: RunOnce(0) exit is %v, not %v", err, kvm.ErrUnexpectedExitReason)
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

func TestCPUtoFD(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	if _, err := m.CPUToFD(0); err != nil {
		t.Errorf("m.CPUtoFD(0): got %v, want nil", err)
	}

	if _, err := m.CPUToFD(42); !errors.Is(err, machine.ErrBadCPU) {
		t.Errorf("m.CPUtoFD(42): got nil, want %v", machine.ErrBadCPU)
	}
}

func TestVtoP(t *testing.T) {
	t.Parallel()

	m, err := machine.New("/dev/kvm", 1, "", "", machine.MinMemSize)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	// Test a bad CPU
	if _, err := m.VtoP(1024, 0); err == nil {
		t.Errorf("m.VtoP(1024, 0): got nil, want err")
	}

	// Test a good address
	pa, err := m.VtoP(0, 0)
	if err != nil || pa != 0 {
		t.Errorf("m.VtoP(0, 0): got (%#x, %v), want 0, nil", pa, err)
	}

	pa, err = m.VtoP(0, 0xf<<56)
	if !errors.Is(err, machine.ErrBadVA) || pa != -1 {
		t.Errorf("m.VtoP(0, 0xf<<56): got (%#x, %v), want -1, %v", pa, err, machine.ErrBadVA)
	}
}

func TestMemTooSmall(t *testing.T) {
	t.Parallel()

	if _, err := machine.New("/dev/kvm", 1, "", "", 1<<16); !errors.Is(err, machine.ErrMemTooSmall) {
		t.Fatalf(`machine.New("/dev/kvm", 1, "", "", 1<<16): got nil, want %v`, machine.ErrMemTooSmall)
	}
}

func TestGetReg(t *testing.T) { // nolint:paralleltest
	regs := []x86asm.Reg{
		x86asm.RAX,
		x86asm.RCX,
		x86asm.RDX,
		x86asm.RBX,
		x86asm.RSP,
		x86asm.RBP,
		x86asm.RSI,
		x86asm.RDI,
		x86asm.R8,
		x86asm.R9,
		x86asm.R10,
		x86asm.R11,
		x86asm.R12,
		x86asm.R13,
		x86asm.R14,
		x86asm.R15,
		x86asm.RIP,
	}
	r := &kvm.Regs{
		RAX: 1,
		RCX: 2,
		RDX: 3,
		RBX: 4,
		RSP: 5,
		RBP: 6,
		RSI: 7,
		RDI: 8,
		R8:  9,
		R9:  10,
		R10: 11,
		R11: 12,
		R12: 13,
		R13: 14,
		R14: 15,
		R15: 16,
		RIP: 17,
	}

	for i := range regs {
		v, err := machine.GetReg(r, regs[i])
		if err != nil {
			t.Errorf("GetReg(r, %#x): got %v, want nil", regs[i], err)
		}

		if *v != uint64(i+1) {
			t.Errorf("Reg %#x: got %#x, want %#x", i, *v, i+1)
		}
	}

	if _, err := machine.GetReg(r, x86asm.AL); err == nil {
		t.Errorf("GetReg(r, x86asm.AL): got nil, want err")
	}
}
