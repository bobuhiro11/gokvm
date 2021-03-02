package pci

// Configuration Space Access Mechanism #1
//
// refs
// https://wiki.osdev.org/PCI
// http://www2.comp.ufscar.br/~helio/boot-int/pci.html
type address uint32

func (a address) getRegisterOffset() uint32 {
	return uint32(a) & 0xff
}

func (a address) getFunctionNumber() uint32 {
	return (uint32(a) >> 8) & 0x7
}

func (a address) getDeviceNumber() uint32 {
	return (uint32(a) >> 11) & 0x1f
}

func (a address) getBusNumber() uint32 {
	return (uint32(a) >> 16) & 0xff
}

func (a address) isEnable() bool {
	return ((uint32(a) >> 31) | 0x1) == 0x1
}

type PCI struct {
	addr address
}

func New() *PCI {
	return &PCI{
		addr: 0xaabbccdd,
	}
}

func (p *PCI) PciConfDataIn(port uint64, values []byte) error {
	if p.addr.getRegisterOffset() == 0 {
		// Vendor ID for virtio PCI
		values[0] = 0xF4
		values[1] = 0x1A
	}
	if p.addr.getRegisterOffset() == 8 {
		// Device ID for virtio PCI
		values[0] = 0x00
		values[1] = 0x10
	}
	return nil
}

func (p *PCI) PciConfDataOut(port uint64, values []byte) error {
	return nil
}

func (p *PCI) PciConfAddrIn(port uint64, values []byte) error {
	if len(values) != 4 {
		return nil
	}

	values[3] = uint8((p.addr >> 24) & 0xff)
	values[2] = uint8((p.addr >> 16) & 0xff)
	values[1] = uint8((p.addr >> 8) & 0xff)
	values[0] = uint8((p.addr >> 0) & 0xff)

	return nil
}

func (p *PCI) PciConfAddrOut(port uint64, values []byte) error {
	if len(values) != 4 {
		return nil
	}

	x := uint32(0)
	x |= uint32(values[3]) << 24
	x |= uint32(values[2]) << 16
	x |= uint32(values[1]) << 8
	x |= uint32(values[0]) << 0

	p.addr = address(x)

	return nil
}
