package pci

import (
	"bytes"
	"encoding/binary"
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

type deviceHeader struct {
	vendorID uint16
	deviceID uint16
	command uint16
	status uint16
	revisonID uint8
	classCode [3]uint8
	cacheLineSize uint8
	latencyTimer uint8
	headerTYpe uint8
	bist uint8
	baseAddressRegister [6]uint32
	cardbusCISPointer uint32
	subsystemVendorID uint16
	subsystemID uint16
	expansionROMBaseAddress uint32
	capabilitiesPointer uint8
	reserved [7]uint8
	interruptLine uint8
	interruptPin uint8
	minGnt uint8
	maxLat uint8
}

func (h *deviceHeader) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type PCI struct {
	addr address
	headers []*deviceHeader
}

func New() *PCI {
	p := &PCI{}

	// 00:00.0 for PCI bridge
	p.headers = append(p.headers, &deviceHeader{
		deviceID: 0x6000,
		vendorID: 0x8086,
		headerTYpe: 1,
	})

	// 00:01.0 for Virtio PCI
	p.headers = append(p.headers, &deviceHeader{
		deviceID: 0x1000,
		vendorID: 0x1AF4,
		headerTYpe: 0,
	})
	return p
}

func (p *PCI) PciConfDataIn(port uint64, values []byte) error {
	// offset can be obtained from many source as below:
	//        (address from IO port 0xcf8) & 0xfc + (IO port address for Data) - 0xCFC
	// see pci_conf1_read in linux/arch/x86/pci/direct.c for more detail.

	offset := int(p.addr.getRegisterOffset() + uint32(port - 0xCFC))

	if p.addr.getBusNumber() != 0 {
		return nil
	}

	if p.addr.getFunctionNumber() != 0 {
		return nil
	}

	slot := int(p.addr.getDeviceNumber())

	if slot >= len(p.headers) {
		return nil
	}

	b, err := p.headers[slot].Bytes()
	if err != nil {
		return err
	}

	for i := 0; i < len(values); i++ {
		values[i] = b[offset+i]
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
