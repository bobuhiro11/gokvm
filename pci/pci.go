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

const (
	hostFeatures  = 0
	guestFeatures = 4
	queuePFN      = 8
	queueNUM      = 12
	queueSEL      = 14
	queueNotify   = 16
	status        = 18
	isr           = 19

	// virtio-net.
	mac               = 20
	netStatus         = 26
	maxVirtQueuePairs = 28
)

func offset2Str(offset uint64) string {
	switch offset {
	case hostFeatures:
		return "hostFeatures"
	case guestFeatures:
		return "guestFeatures"
	case queuePFN:
		return "queuePFN"
	case queueNUM:
		return "queueNUM"
	case queueSEL:
		return "queueSEL"
	case queueNotify:
		return "queueNotify"
	case status:
		return "status"
	case isr:
		return "isr"
	case mac:
		return "mac"
	case netStatus:
		return "netStatus"
	case maxVirtQueuePairs:
		return "maxVirtQueuePairs"
	default:
		return ""
	}
}

// type virtioHeader struct {
// 	// common
// 	hostFeatures uint32
// 	guestFeatures uint32
// 	queuePFN uint32
// 	queueNUM uint16
// 	queueSEL uint16
// 	queueNotify uint16
// 	status uint8
// 	isr uint8
//
// 	// virtio-net
// 	mac [6]uint8
// 	netStatus uint16
// 	maxVirtQueuePairs uint16
// }

type deviceHeader struct {
	vendorID      uint16
	deviceID      uint16
	command       uint16
	_             uint16   // status
	_             uint8    // revisonID
	_             [3]uint8 // classCode
	_             uint8    // cacheLineSize
	_             uint8    // latencyTimer
	headerType    uint8
	_             uint8 // bist
	bar           [6]uint32
	_             uint32 // cardbusCISPointer
	_             uint16 // subsystemVendorID
	subsystemID   uint16
	_             uint32   // expansionROMBaseAddress
	_             uint8    // capabilitiesPointer
	_             [7]uint8 // reserved
	interruptLine uint8
	interruptPin  uint8
	_             uint8 // minGnt
	_             uint8 // maxLat
}

func (h *deviceHeader) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type PCI struct {
	addr    address
	headers []*deviceHeader
	// virtioHeader *virtioHeader
}

const (
	MMIOStart     = 0x3f000000
	IOportStart   = 0x6200
	PciIOSize     = 0x100
	pciIOSizeBits = uint32(0xffffff00)
	barMem        = 0x0
	barIO         = 0x1
)

func New() *PCI {
	p := &PCI{}

	// 00:00.0 for PCI bridge
	p.headers = append(p.headers, &deviceHeader{
		deviceID:   0x6000,
		vendorID:   0x8086,
		headerType: 1,
	})

	// 00:01.0 for Virtio PCI
	p.headers = append(p.headers, &deviceHeader{
		deviceID:   0x1000,
		vendorID:   0x1AF4,
		headerType: 0,
		bar: [6]uint32{
			IOportStart | barIO,
			MMIOStart | barMem,
		},
		subsystemID: 1,         // network card
		command:     0x1 | 0x2, // IO_EN, MEM_EN
		// https://github.com/torvalds/linux/blob/fb3b0673b7d5b477ed104949450cd511337ba3c6/drivers/pci/setup-irq.c#L30-L55
		interruptPin: 1,

		// https://www.webopedia.com/reference/irqnumbers/
		interruptLine: 9,
	})
	// p.virtioHeader = &virtioHeader{}

	return p
}

func (p *PCI) VirtioIn(port uint64, values []byte) error {
	offset := port - IOportStart

	if offset == queueNUM {
		values[0] = 0x10
		values[1] = 0x00
	}

	fmt.Printf("VirtioIn offset:0x%x(%s) values:%#v\r\n", offset, offset2Str(offset), values)

	return nil
}

func (p *PCI) VirtioOut(port uint64, values []byte) error {
	offset := port - IOportStart
	fmt.Printf("VirtioOut offset:0x%x(%s) values:%#v\r\n", offset, offset2Str(offset), values)

	if offset == queuePFN {
		x := uint32(0)
		x |= uint32(values[3]) << 24
		x |= uint32(values[2]) << 16
		x |= uint32(values[1]) << 8
		x |= uint32(values[0]) << 0
		x *= 4096 // 4KB page size
		fmt.Printf("guest phys mem addr for queue: 0x%x\r\n", x)
	}

	return nil
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

	if slot >= len(p.headers) {
		return nil
	}

	b, err := p.headers[slot].Bytes()
	if err != nil {
		return err
	}

	l := len(values)
	copy(values[:l], b[offset:offset+l])

	fmt.Printf("PciConfDataIn: port:%#v values:%#v\r\n", port, values)

	return nil
}

func (p *PCI) PciConfDataOut(port uint64, values []byte) error {
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

	if slot >= len(p.headers) {
		return nil
	}

	bar := offset/4 - 4
	if slot == 1 && (bar == 0 || bar == 1) && len(values) == 4 &&
		values[0] == 0xff && values[1] == 0xff && values[2] == 0xff && values[3] == 0xff { // BAR0 or BAR1 for slot1
		x := uint32(0)
		x |= uint32(values[3]) << 24
		x |= uint32(values[2]) << 16
		x |= uint32(values[1]) << 8
		x |= uint32(values[0]) << 0

		if x == 0xffffffff { // init process
			p.headers[slot].bar[bar] = pciIOSizeBits
		} else {
			p.headers[slot].bar[bar] = x
		}
	}

	fmt.Printf("PciConfDataOut: port:%#v values:%#v\r\n", port, values)

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
