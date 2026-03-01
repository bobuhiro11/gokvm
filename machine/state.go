package machine

// state.go – VM snapshot helpers for live migration.
// Each Save* method captures state into migration.* types.
// Each Restore* method applies previously captured state back.

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/migration"
)

// structBytes returns a byte slice that aliases the memory of v.
// v must be a pointer to a fixed-size struct.
func structBytes[T any](v *T) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(v)), unsafe.Sizeof(*v))
}

// copyStruct fills *dst from a byte slice produced by structBytes.
func copyStruct[T any](dst *T, b []byte) error {
	size := int(unsafe.Sizeof(*dst))
	if len(b) < size {
		return fmt.Errorf("state buffer too small: got %d want %d", len(b), size)
	}

	copy(unsafe.Slice((*byte)(unsafe.Pointer(dst)), size), b[:size])

	return nil
}

// cloneBytes returns a copy of s as a new slice.
func cloneBytes(s []byte) []byte {
	c := make([]byte, len(s))
	copy(c, s)

	return c
}

// msrIndexList retrieves the list of MSR indices supported by this KVM instance.
func (m *Machine) msrIndexList() ([]uint32, error) {
	list := &kvm.MSRList{}

	// First call: E2BIG tells us how many entries are available.
	err := kvm.GetMSRIndexList(m.kvmFd, list)
	if !errors.Is(err, syscall.E2BIG) && err != nil {
		return nil, fmt.Errorf("GetMSRIndexList probe: %w", err)
	}

	// Second call: the list is now sized correctly.
	if err := kvm.GetMSRIndexList(m.kvmFd, list); err != nil {
		return nil, fmt.Errorf("GetMSRIndexList fetch: %w", err)
	}

	indices := make([]uint32, list.NMSRs)
	copy(indices, list.Indicies[:list.NMSRs])

	return indices, nil
}

// SaveCPUState captures the full architectural state of one vCPU.
func (m *Machine) SaveCPUState(cpu int) (*migration.VCPUState, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return nil, err
	}

	state := &migration.VCPUState{}

	// General-purpose registers.
	regs, err := kvm.GetRegs(fd)
	if err != nil {
		return nil, fmt.Errorf("GetRegs cpu%d: %w", cpu, err)
	}

	state.Regs = cloneBytes(structBytes(regs))

	// Control / segment registers.
	sregs, err := kvm.GetSregs(fd)
	if err != nil {
		return nil, fmt.Errorf("GetSregs cpu%d: %w", cpu, err)
	}

	state.Sregs = cloneBytes(structBytes(sregs))

	// Model-specific registers.
	indices, err := m.msrIndexList()
	if err != nil {
		return nil, err
	}

	msrs := &kvm.MSRS{
		NMSRs:   uint32(len(indices)),
		Entries: make([]kvm.MSREntry, len(indices)),
	}

	for i, idx := range indices {
		msrs.Entries[i].Index = idx
	}

	if err := kvm.GetMSRs(fd, msrs); err != nil {
		return nil, fmt.Errorf("GetMSRs cpu%d: %w", cpu, err)
	}

	state.MSRs = make([]migration.MSREntry, len(msrs.Entries))
	for i, e := range msrs.Entries {
		state.MSRs[i] = migration.MSREntry{Index: e.Index, Data: e.Data}
	}

	// Local APIC.
	lapic := &kvm.LAPICState{}
	if err := kvm.GetLocalAPIC(fd, lapic); err != nil {
		return nil, fmt.Errorf("GetLocalAPIC cpu%d: %w", cpu, err)
	}

	state.LAPIC = cloneBytes(structBytes(lapic))

	// Pending exceptions / interrupts.
	events := &kvm.VCPUEvents{}
	if err := kvm.GetVCPUEvents(fd, events); err != nil {
		return nil, fmt.Errorf("GetVCPUEvents cpu%d: %w", cpu, err)
	}

	state.Events = cloneBytes(structBytes(events))

	// Multiprocessor state.
	mps := &kvm.MPState{}
	if err := kvm.GetMPState(fd, mps); err != nil {
		return nil, fmt.Errorf("GetMPState cpu%d: %w", cpu, err)
	}

	state.MPState = mps.State

	// Debug registers.
	dregs := &kvm.DebugRegs{}
	if err := kvm.GetDebugRegs(fd, dregs); err != nil {
		return nil, fmt.Errorf("GetDebugRegs cpu%d: %w", cpu, err)
	}

	state.DebugRegs = cloneBytes(structBytes(dregs))

	// Extended control registers (AVX/SSE state).
	xcrs := &kvm.XCRS{}
	if err := kvm.GetXCRS(fd, xcrs); err != nil {
		return nil, fmt.Errorf("GetXCRS cpu%d: %w", cpu, err)
	}

	state.XCRS = cloneBytes(structBytes(xcrs))

	return state, nil
}

// SaveVMState captures VM-level (non-per-vCPU) hardware state.
func (m *Machine) SaveVMState() (*migration.VMState, error) {
	state := &migration.VMState{}

	// KVM clock (kvmclock) — must be saved for monotonicity.
	cd := &kvm.ClockData{}
	if err := kvm.GetClock(m.vmFd, cd); err != nil {
		return nil, fmt.Errorf("GetClock: %w", err)
	}

	state.Clock = cloneBytes(structBytes(cd))

	// IRQ chip: master PIC (0), slave PIC (1), IOAPIC (2).
	for chipID, dest := range [](*[]byte){&state.IRQChipPIC0, &state.IRQChipPIC1, &state.IRQChipIOAPIC} {
		chip := &kvm.IRQChip{ChipID: uint32(chipID)}
		if err := kvm.GetIRQChip(m.vmFd, chip); err != nil {
			return nil, fmt.Errorf("GetIRQChip(%d): %w", chipID, err)
		}

		*dest = cloneBytes(structBytes(chip))
	}

	// PIT (programmable interval timer).
	pit := &kvm.PITState2{}
	if err := kvm.GetPIT2(m.vmFd, pit); err != nil {
		return nil, fmt.Errorf("GetPIT2: %w", err)
	}

	state.PIT2 = cloneBytes(structBytes(pit))

	return state, nil
}

// RestoreVMState applies previously saved VM-level hardware state.
func (m *Machine) RestoreVMState(state *migration.VMState) error {
	// KVM clock.
	var cd kvm.ClockData
	if err := copyStruct(&cd, state.Clock); err != nil {
		return fmt.Errorf("decode ClockData: %w", err)
	}

	if err := kvm.SetClock(m.vmFd, &cd); err != nil {
		return fmt.Errorf("SetClock: %w", err)
	}

	// IRQ chips.
	for _, src := range [][]byte{state.IRQChipPIC0, state.IRQChipPIC1, state.IRQChipIOAPIC} {
		var chip kvm.IRQChip
		if err := copyStruct(&chip, src); err != nil {
			return fmt.Errorf("decode IRQChip: %w", err)
		}

		if err := kvm.SetIRQChip(m.vmFd, &chip); err != nil {
			return fmt.Errorf("SetIRQChip(%d): %w", chip.ChipID, err)
		}
	}

	// PIT.
	var pit kvm.PITState2
	if err := copyStruct(&pit, state.PIT2); err != nil {
		return fmt.Errorf("decode PITState2: %w", err)
	}

	if err := kvm.SetPIT2(m.vmFd, &pit); err != nil {
		return fmt.Errorf("SetPIT2: %w", err)
	}

	return nil
}

// RestoreCPUState applies a previously saved vCPU state.
func (m *Machine) RestoreCPUState(cpu int, state *migration.VCPUState) error {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return err
	}

	// General-purpose registers.
	var regs kvm.Regs
	if err := copyStruct(&regs, state.Regs); err != nil {
		return fmt.Errorf("decode Regs cpu%d: %w", cpu, err)
	}

	if err := kvm.SetRegs(fd, &regs); err != nil {
		return fmt.Errorf("SetRegs cpu%d: %w", cpu, err)
	}

	// Control / segment registers.
	var sregs kvm.Sregs
	if err := copyStruct(&sregs, state.Sregs); err != nil {
		return fmt.Errorf("decode Sregs cpu%d: %w", cpu, err)
	}

	if err := kvm.SetSregs(fd, &sregs); err != nil {
		return fmt.Errorf("SetSregs cpu%d: %w", cpu, err)
	}

	// Model-specific registers.
	msrs := &kvm.MSRS{
		NMSRs:   uint32(len(state.MSRs)),
		Entries: make([]kvm.MSREntry, len(state.MSRs)),
	}

	for i, e := range state.MSRs {
		msrs.Entries[i].Index = e.Index
		msrs.Entries[i].Data = e.Data
	}

	if err := kvm.SetMSRs(fd, msrs); err != nil {
		return fmt.Errorf("SetMSRs cpu%d: %w", cpu, err)
	}

	// Local APIC.
	var lapic kvm.LAPICState
	if err := copyStruct(&lapic, state.LAPIC); err != nil {
		return fmt.Errorf("decode LAPIC cpu%d: %w", cpu, err)
	}

	if err := kvm.SetLocalAPIC(fd, &lapic); err != nil {
		return fmt.Errorf("SetLocalAPIC cpu%d: %w", cpu, err)
	}

	// Pending exceptions / interrupts.
	var events kvm.VCPUEvents
	if err := copyStruct(&events, state.Events); err != nil {
		return fmt.Errorf("decode VCPUEvents cpu%d: %w", cpu, err)
	}

	if err := kvm.SetVCPUEvents(fd, &events); err != nil {
		return fmt.Errorf("SetVCPUEvents cpu%d: %w", cpu, err)
	}

	// Multiprocessor state.
	mps := kvm.MPState{State: state.MPState}
	if err := kvm.SetMPState(fd, &mps); err != nil {
		return fmt.Errorf("SetMPState cpu%d: %w", cpu, err)
	}

	// Debug registers.
	var dregs kvm.DebugRegs
	if err := copyStruct(&dregs, state.DebugRegs); err != nil {
		return fmt.Errorf("decode DebugRegs cpu%d: %w", cpu, err)
	}

	if err := kvm.SetDebugRegs(fd, &dregs); err != nil {
		return fmt.Errorf("SetDebugRegs cpu%d: %w", cpu, err)
	}

	// Extended control registers.
	var xcrs kvm.XCRS
	if err := copyStruct(&xcrs, state.XCRS); err != nil {
		return fmt.Errorf("decode XCRS cpu%d: %w", cpu, err)
	}

	if err := kvm.SetXCRS(fd, &xcrs); err != nil {
		return fmt.Errorf("SetXCRS cpu%d: %w", cpu, err)
	}

	return nil
}
