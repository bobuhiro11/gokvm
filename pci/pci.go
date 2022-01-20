package pci

import (
	"bytes"
	"encoding/binary"
)

// Configuration Space Access Mechanism #1
//
// refs
// https://wiki.osdev.org/PCI
// http://www2.comp.ufscar.br/~helio/boot-int/pci.html
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

// interface for a PCI device.
type Device interface {
	GetDeviceHeader() DeviceHeader
	IOInHandler(port uint64, bytes []byte) error
	IOOutHandler(port uint64, bytes []byte) error

	// IO port range for this PCI device.
	// This range corresponds to IO Range in BAR0.
	GetIORange() (start, end uint64)
}

type DeviceHeader struct {
	VendorID   uint16
	DeviceID   uint16
	_          uint16   // command
	_          uint16   // status
	_          uint8    // revisonID
	_          [3]uint8 // classCode
	_          uint8    // cacheLineSize
	_          uint8    // latencyTimer
	HeaderType uint8
	_          uint8     // bist
	_          [6]uint32 // baseAddressRegister
	_          uint32    // cardbusCISPointer
	_          uint16    // subsystemVendorID
	_          uint16    // subsystemID
	_          uint32    // expansionROMBaseAddress
	_          uint8     // capabilitiesPointer
	_          [7]uint8  // reserved
	_          uint8     // interruptLine
	_          uint8     // interruptPin
	_          uint8     // minGnt
	_          uint8     // maxLat
}

func (h DeviceHeader) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type PCI struct {
	addr    address
	Devices []Device
}

func New(devices ...Device) *PCI {
	return &PCI{Devices: devices}
}

func (p *PCI) PciConfDataIn(port uint64, values []byte) error {
	// offset can be obtained from many source as below:
	//        (address from IO port 0xcf8) & 0xfc + (IO port address for Data) - 0xCFC
	// see pci_conf1_read in linux/arch/x86/pci/direct.c for more detail.
	offset := int(p.addr.getRegisterOffset() + uint32(port-0xCFC))

	if !p.addr.isEnable() {
		return nil
	}

	if p.addr.getBusNumber() != 0 {
		return nil
	}

	if p.addr.getFunctionNumber() != 0 {
		return nil
	}

	slot := int(p.addr.getDeviceNumber())

	if slot >= len(p.Devices) {
		return nil
	}

	b, err := p.Devices[slot].GetDeviceHeader().Bytes()
	if err != nil {
		return err
	}

	l := len(values)
	copy(values[:l], b[offset:offset+l])

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
