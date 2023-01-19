package machine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"

	"github.com/bobuhiro11/gokvm/kvm"
	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/sys/unix"
)

// Debug is a normally empty function that enables debug prints.
// well too bad. var debug = log.Printf // func(string, ...interface{}) {}

// ErrBadRegister indicates a bad register was used.
var ErrBadRegister = errors.New("bad register")

// Args returns the top nargs args, going down the stack if needed. The max is 6.
// This is UEFI calling convention.
func (m *Machine) Args(cpu int, r *syscall.PtraceRegs, nargs int) []uintptr {
	sp := uintptr(r.Rsp)

	switch nargs {
	case 6:
		w1, _ := m.ReadWord(cpu, sp+0x28)
		w2, _ := m.ReadWord(cpu, sp+0x30)

		return []uintptr{uintptr(r.Rcx), uintptr(r.Rdx), uintptr(r.R8), uintptr(r.R9), uintptr(w1), uintptr(w2)}
	case 5:
		w1, _ := m.ReadWord(cpu, sp+0x28)

		return []uintptr{uintptr(r.Rcx), uintptr(r.Rdx), uintptr(r.R8), uintptr(r.R9), uintptr(w1)}
	case 4:
		return []uintptr{uintptr(r.Rcx), uintptr(r.Rdx), uintptr(r.R8), uintptr(r.R9)}
	case 3:
		return []uintptr{uintptr(r.Rcx), uintptr(r.Rdx), uintptr(r.R8)}
	case 2:
		return []uintptr{uintptr(r.Rcx), uintptr(r.Rdx)}
	case 1:
		return []uintptr{uintptr(r.Rcx)}
	}

	return []uintptr{}
}

// Pointer returns the data pointed to by args[arg].
func (m *Machine) Pointer(inst *x86asm.Inst, r *kvm.Regs, arg int) (uintptr, error) {
	mem := inst.Args[arg].(x86asm.Mem)
	// A Mem is a memory reference.
	// The general form is Segment:[Base+Scale*Index+Disp].
	/*
		type Mem struct {
			Segment Reg
			Base    Reg
			Scale   uint8
			Index   Reg
			Disp    int64
		}
	*/
	// debug("ARG[%d] %q m is %#x", inst.Args[arg], mem)

	b, err := GetReg(r, mem.Base)
	if err != nil {
		return 0, fmt.Errorf("base reg %v in %v:%w", mem.Base, mem, ErrBadRegister)
	}

	addr := *b + uint64(mem.Disp)

	x, err := GetReg(r, mem.Index)
	if err == nil {
		addr += uint64(mem.Scale) * (*x)
	}

	// if v, ok := inst.Args[0].(*x86asm.Mem); ok {
	// debug("computed addr is %#x", addr)

	return uintptr(addr), nil
}

// Pop pops the stack and returns what was at TOS.
// It is most often used to get the caller PC (cpc).
func (m *Machine) Pop(cpu int, r *kvm.Regs) (uint64, error) {
	cpc, err := m.ReadWord(cpu, uintptr(r.RSP))
	if err != nil {
		return 0, err
	}

	r.RSP += 8

	return cpc, nil
}

// Inst retrieves an instruction from the guest, at RIP.
// It returns an x86asm.Inst, Ptraceregs, a string in GNU syntax, and
// and error.
func (m *Machine) Inst(cpu int) (*x86asm.Inst, *kvm.Regs, string, error) {
	r, err := m.GetRegs(cpu)
	if err != nil {
		return nil, nil, "", fmt.Errorf("Inst:Getregs:%w", err)
	}

	pc := uintptr(r.RIP)

	// debug("Inst: pc %#x, sp %#x", pc, sp)
	// We know the PC; grab a bunch of bytes there, then decode and print
	insn := make([]byte, 16)
	if _, err := m.ReadBytes(cpu, insn, pc); err != nil {
		return nil, nil, "", fmt.Errorf("reading PC at #%x:%w", pc, err)
	}

	d, err := x86asm.Decode(insn, 64)
	if err != nil {
		return nil, nil, "", fmt.Errorf("decoding %#02x:%w", insn, err)
	}

	return &d, &r, x86asm.GNUSyntax(d, r.RIP, nil), nil
}

// Asm returns a string for the given instruction at the given pc.
func Asm(d *x86asm.Inst, pc uint64) string {
	return "\"" + x86asm.GNUSyntax(*d, pc, nil) + "\""
}

// CallInfo provides calling info for a function.
func CallInfo(_ *unix.SignalfdSiginfo, inst *x86asm.Inst, r *kvm.Regs) string {
	l := fmt.Sprintf("%s[", show("", r))
	for _, a := range inst.Args {
		l += fmt.Sprintf("%v,", a)
	}

	l += fmt.Sprintf("(%#x, %#x, %#x, %#x)", r.RCX, r.RDX, r.R8, r.R9)

	return l
}

// WriteWord writes the given word into the guest's virtual address space.
func (m *Machine) WriteWord(cpu int, vaddr uintptr, word uint64) error {
	pa, err := m.VtoP(cpu, vaddr)
	if err != nil {
		return err
	}

	var b [8]byte

	binary.LittleEndian.PutUint64(b[:], word)
	_, err = m.WriteAt(b[:], pa)

	return err
}

// ReadWord reads bytes from the CPUs virtual address space.
func (m *Machine) ReadBytes(cpu int, b []byte, vaddr uintptr) (int, error) {
	pa, err := m.VtoP(cpu, vaddr)
	if err != nil {
		return -1, err
	}

	return m.ReadAt(b, pa)
}

// ReadWord reads the given word from the cpu's virtual address space.
func (m *Machine) ReadWord(cpu int, vaddr uintptr) (uint64, error) {
	var b [8]byte
	if _, err := m.ReadBytes(cpu, b[:], vaddr); err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(b[:]), nil
}
