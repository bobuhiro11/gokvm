package pci

import (
	"fmt"
)

// Configuration Space Access Mechanism #1
//
// refs
// https://wiki.osdev.org/PCI
// http://www2.comp.ufscar.br/~helio/boot-int/pci.html
// nolint:gochecknoglobals

type address uint32

func (a address) getRegisterOffset() uint32 {
	return uint32(a) & 0xfc
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
	// offset can be obtained from many source as below:
	//        (address from IO port 0xcf8) & 0xfc + (IO port address for Data) - 0xCFC
	// see pci_conf1_read in linux/arch/x86/pci/direct.c for more detail.

	offset := p.addr.getRegisterOffset() + uint32(port - 0xCFC)
	if offset == 0x0a { // PCI_CLASS_DEVICE
		values[0] = 0x00 // PCI_CLASS_BRIDGE_HOST
		values[1] = 0x60
	}

	if offset == 0x00 { // PCI_VENDOR_ID
		values[0] = 0x86 // PCI_VENDOR_ID_INTEL
		values[1] = 0x80
	}

	fmt.Printf("PciConfDataIn: offset:0x%x values: %#v\r\n", offset, values)
	return nil
}

func (p *PCI) PciConfDataOut(port uint64, values []byte) error {
	fmt.Printf("PciConfDataOut: values: %#v\r\n", values)
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

	fmt.Printf("PciConfAddrIn: port=0x%x x=0x%x slot=%d func=%d\r\n", port, p.addr,
		p.addr.getDeviceNumber(),
		p.addr.getFunctionNumber())

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

	fmt.Printf("PciConfAddrOut: port=0x%x x=0x%x slot=%d func=%d\r\n", port, p.addr,
		p.addr.getDeviceNumber(),
		p.addr.getFunctionNumber())

	return nil
}
