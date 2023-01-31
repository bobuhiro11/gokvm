package kvm

import "unsafe"

// irqLevel defines an IRQ as Level? Not sure.
type irqLevel struct {
	IRQ   uint32
	Level uint32
}

// IRQLines sets the interrupt line for an IRQ.
func IRQLine(vmFd uintptr, irq, level uint32) error {
	irqLev := irqLevel{
		IRQ:   irq,
		Level: level,
	}

	_, err := Ioctl(vmFd, kvmIRQLine, uintptr(unsafe.Pointer(&irqLev)))

	return err
}

// CreateIRQChip creates an IRQ device (chip) to which to attach interrupts?
func CreateIRQChip(vmFd uintptr) error {
	_, err := Ioctl(vmFd, kvmCreateIRQChip, 0)

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
	_, err := Ioctl(vmFd, kvmCreatePIT2, uintptr(unsafe.Pointer(&pit)))

	return err
}
