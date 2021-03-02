package pci

import (
	"fmt"
)

// refs
// https://wiki.osdev.org/PCI
// http://www2.comp.ufscar.br/~helio/boot-int/pci.html
// nolint:gochecknoglobals
var (
	AddrPorts = []int{0xcf8, 0xcf9, 0xcfa, 0xcfb}
	DataPorts = []int{0xcfc, 0xcfd, 0xcfe, 0xcff}
)

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
	return &PCI{}
}

func (p *PCI) In(port uint64, values []byte) error {
	if len(values) != 2 || p.addr.getBusNumber() != 0 || p.addr.getDeviceNumber() != 3 || p.addr.getFunctionNumber() != 0 {
		for i := 0; i < len(values); i++ {
			values[i] = 0xff
		}

		return nil
	}

	// vendor id: intel 0x8086
	if p.addr.getRegisterOffset() == 0x0 {
		values[0] = 0x86
		values[1] = 0x80
	}

	// device id: 82542 Gigabit Ethernet Controller (Fiber)
	if p.addr.getRegisterOffset() == 0x2 {
		values[0] = 0x00
		values[1] = 0x10
	}

	// class code & subclass
	// ethernet controller
	if p.addr.getRegisterOffset() == 0xa {
		values[0] = 0x00 // sub class
		values[1] = 0x02 // class code
	}

	// header type
	if p.addr.getRegisterOffset() == 0xe {
		values[0] = 0x00 // header type
		values[1] = 0x00 // BIST
	}

	fmt.Printf("PCI  IN: port=0x%x values=%v\r\n", port, values)

	return nil
}

func (p *PCI) Out(port uint64, values []byte) error {
	for i := range AddrPorts {
		if int(port) != AddrPorts[i] {
			continue
		}

		if len(values) != 4 {
			continue
		}

		if port != 0x0cf8 {
			continue
		}

		p.addr = 0
		p.addr |= address(values[3]) << 24
		p.addr |= address(values[2]) << 16
		p.addr |= address(values[1]) << 8
		p.addr |= address(values[0]) << 0

		if p.addr.getBusNumber() == 0 && p.addr.getDeviceNumber() == 3 && p.addr.getFunctionNumber() == 0 {
			fmt.Printf("pci address = %x:%x:%x, offset = 0x%x, enabled = %v\r\n",
				p.addr.getBusNumber(),
				p.addr.getDeviceNumber(),
				p.addr.getFunctionNumber(),
				p.addr.getRegisterOffset(),
				p.addr.isEnable())
		}
	}

	return nil
}
