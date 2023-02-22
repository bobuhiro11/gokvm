package kvm

import (
	"unsafe"
)

// irqLevel defines an IRQ as Level? Not sure.
type irqLevel struct {
	IRQ   uint32
	Level uint32
}

// IRQLines sets the interrupt line for an IRQ.
func IRQLineStatus(vmFd uintptr, irq, level uint32) error {
	irqLev := irqLevel{
		IRQ:   irq,
		Level: level,
	}
	_, err := Ioctl(vmFd,
		IIOWR(kvmIRQLineStatus, unsafe.Sizeof(irqLevel{})),
		uintptr(unsafe.Pointer(&irqLev)))

	return err
}

// CreateIRQChip creates an IRQ device (chip) to which to attach interrupts?
func CreateIRQChip(vmFd uintptr) error {
	_, err := Ioctl(vmFd, IIO(kvmCreateIRQChip), 0)

	return err
}

// pitConfig defines properties of a programmable interrupt timer.
type pitConfig struct {
	Flags uint32
	_     [15]uint32
}

// CreatePIT2 creates a PIT type 2. Just having one was not enough.
func CreatePIT2(vmFd uintptr) error {
	pit := pitConfig{
		Flags: 0,
	}
	_, err := Ioctl(vmFd,
		IIOW(kvmCreatePIT2, unsafe.Sizeof(pitConfig{})),
		uintptr(unsafe.Pointer(&pit)))

	return err
}

type PITChannelState struct {
	Count         uint32
	LatchedCount  uint16
	CountLatched  uint8
	StatusLatched uint8
	Status        uint8
	ReadState     uint8
	WriteState    uint8
	WriteLatch    uint8
	RWMode        uint8
	Mode          uint8
	BCD           uint8
	Gate          uint8
	CountLoadTime int64
}

type PITState2 struct {
	Channels [3]PITChannelState
	Flags    uint32
	_        [9]uint32
}

// GetPIT2 retrieves the state of the in-kernel PIT model. Only valid after KVM_CREATE_PIT2.
func GetPIT2(vmFd uintptr, pstate *PITState2) error {
	_, err := Ioctl(vmFd,
		IIOR(kvmGetPIT2, unsafe.Sizeof(PITState2{})),
		uintptr(unsafe.Pointer(pstate)))

	return err
}

// SetPIT2 sets the state of the in-kernel PIT model. Only valid after KVM_CREATE_PIT2.
func SetPIT2(vmFd uintptr, pstate *PITState2) error {
	_, err := Ioctl(vmFd,
		IIOW(kvmSetPIT2, unsafe.Sizeof(PITState2{})),
		uintptr(unsafe.Pointer(pstate)))

	return err
}
