package machine_test

import (
	"reflect"
	"testing"

	"github.com/bobuhiro11/gokvm/machine"
)

func TestDebug(t *testing.T) { // nolint:paralleltest
	m, err := machine.New("/dev/kvm", 1, 1<<29)
	if err != nil {
		t.Fatalf("Open: got %v, want nil", err)
	}

	rip := uint64(0x1_000_000)
	if err := m.SetupRegs(rip, 0x10_000, false); err != nil {
		t.Fatalf("SetupRegs: got %v, want nil", err)
	}

	r, err := m.GetRegs(0)
	if err != nil {
		t.Fatalf("GetRegs: got %v, want nil", err)
	}

	r.RCX, r.RDX, r.R8, r.R9, r.RSP = 1, 2, 3, 4, 0x1_000_000
	if err := m.SetRegs(0, r); err != nil {
		t.Fatalf("SetRegs: got %v, want nil", err)
	}

	t.Logf("Regs %#x r.RIP %#x", r, r.RIP)

	if r.RIP != rip {
		t.Fatalf("TestDebug: r.RIP is %#x, want %#x", r.RIP, rip)
	}

	rsp := uintptr(rip)

	t.Logf("r.RIP %#x, r.RSP %#x", r.RIP, r.RSP)

	if err := m.WriteWord(0, rsp+0x28, 5); err != nil {
		t.Fatalf("WriteWord(0, %#x, 5): %v != nil", rsp+0x28, err)
	}

	if v, err := m.ReadWord(0, rsp+0x28); err != nil || v != 5 {
		t.Fatalf("ReadWord(0, %#x): got (%d, %v), want (5, nil)", rsp+0x28, v, err)
	}

	if err := m.WriteWord(0, rsp+0x30, 6); err != nil {
		t.Fatalf("WriteWord(0, %#x, 6): %v != nil", rsp+0x28, err)
	}

	if v, err := m.ReadWord(0, rsp+0x30); err != nil || v != 6 {
		t.Fatalf("ReadWord(0, %#x): got (%d, %v), want (6, nil)", rsp+0x28, v, err)
	}

	if _, err := m.Args(1024, r, 1); err == nil {
		t.Errorf("m.Args(1024, ...): got nil, want err")
	}

	if _, err := m.Args(0, r, 800); err == nil {
		t.Errorf("m.Args(0, r, 800): got nil, want err")
	}

	args := []uintptr{1, 2, 3, 4, 5, 6}
	// Just run the Arg code.
	for i := 1; i < 7; i++ {
		a, err := m.Args(0, r, i)
		if err != nil {
			t.Errorf("m.Args(0, r, %d): %v != nil", i, err)
		}

		if !reflect.DeepEqual(a[:i], args[:i]) {
			t.Errorf("m.Args(0, r, %d): got %#x, want %#x, r.RSP %#x", i, a[:i], args[:i], r.RSP)
		}
	}

	if _, _, _, err := m.Inst(1024); err == nil {
		t.Errorf("m.Inst(1024): got nil, want err")
	}

	i, r, s, err := m.Inst(0)
	if err != nil {
		t.Errorf("m.Inst(0): got nil, want err")
	}

	t.Logf("%v %v %v", i, r, s)

	// See if it looks like Poison
	s = machine.Asm(i, 0x1_000_000)
	t.Logf("s at 0x1_000_000 is %s", s)

	if _, err := m.Pointer(i, r, 1024); err == nil {
		t.Errorf("m.Pointer(arg 1024): got nil, want error")
	}

	if _, err := m.Pointer(i, r, 1); err == nil {
		t.Errorf("m.Pointer(i, r, 1): got nil, want err")
	}

	// Sadly, don't have a use for Pointer (yet)

	if _, err := m.Pop(1024, r); err == nil {
		t.Errorf("Pop(1024,...): got nil, want err")
	}

	tos, err := m.ReadWord(0, rsp)
	if err != nil {
		t.Fatalf("ReadWord(0, %#x): got %v, want nil", rsp, err)
	}

	if v, err := m.Pop(0, r); err != nil || v != tos {
		t.Errorf("Pop(0, r): got (%#x, %v), want (%#x, nil)", v, err, tos)
	}

	// Not sure what to test, but call it anyway
	t.Logf("CallInfo: %s", machine.CallInfo(i, r))
}
