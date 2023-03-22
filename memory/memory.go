package memory

import (
	"errors"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/kvm"
)

var (
	errNoSlotsAvail         = errors.New("maximal numbers of slots exhausted")
	errSlotNotFound         = errors.New("unable to find MemorySlot")
	errAddressSpaceNotFound = errors.New("unable to find address space")
)

const (
	// Poison is an instruction that should force a vmexit.
	// it fills memory to make catching guest errors easier.
	// vmcall, nop is this pattern
	// Poison = []byte{0x0f, 0x0b, } //0x01, 0xC1, 0x90}
	// Disassembly:
	// 0:  b8 be ba fe ca          mov    eax,0xcafebabe
	// 5:  90                      nop
	// 6:  0f 0b                   ud2
	Poison = "\xB8\xBE\xBA\xFE\xCA\x90\x0F\x0B"

	highMemBase = 0x100000
)

type RegionType uint8

const (
	RAM RegionType = 0 + iota
	ROM
	IO
)

type Memory struct {
	Slots    []*MemorySlot
	MaxSlots uint32
}

type MemorySlot struct {
	Addr          uint64
	Size          int
	Slot          uint8
	Flags         uint32
	OldFlags      uint32
	DirtyBMap     uint32
	DirtyBMapSize uint32
	PhysAddr      uint64
	AS            *AddressSpace
	Buf           []byte
}

func New(kvmfd uintptr, ramsize int) (*Memory, error) {
	as := NewAddressSpace("phys-ram", 0, uint32(ramsize))
	mgnt := &Memory{}

	ret, err := kvm.CheckExtension(kvmfd, kvm.CapNRMemSlots)
	if err != nil {
		return nil, err
	}

	if ret <= 0 {
		return nil, err
	}

	mgnt.MaxSlots = uint32(ret)

	if err := mgnt.NewMemorySlot(0, ramsize, 0, as); err != nil {
		return nil, err
	}

	return mgnt, nil
}

func (m *Memory) FindSlot(addr uint64, size int) (*MemorySlot, error) {
	for _, slot := range m.Slots {
		if slot.Addr == addr && slot.Size == size {
			return slot, nil
		}
	}

	return nil, errSlotNotFound
}

func (m *Memory) NewMemorySlot(addr uint64, size int, flags uint32, as *AddressSpace) error {
	var err error

	if len(m.Slots) >= int(m.MaxSlots) {
		return errNoSlotsAvail
	}

	slot := &MemorySlot{
		Addr:  addr,
		Size:  size,
		Flags: flags,
		AS:    as,
	}

	slot.Buf, err = syscall.Mmap(-1, 0, size, syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		return err
	}

	// Poison memory.
	// 0 is valid instruction and if you start running in the middle of all those
	// 0's it is impossible to diagnore.
	for i := highMemBase; i < len(slot.Buf); i += len(Poison) {
		copy(slot.Buf[i:], Poison)
	}

	slot.PhysAddr = uint64(uintptr(unsafe.Pointer(&slot.Buf[0])))

	m.Slots = append(m.Slots, slot)

	return nil
}
