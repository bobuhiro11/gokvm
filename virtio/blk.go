package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"github.com/bobuhiro11/gokvm/pci"
)

const (
	BlkIOPortStart = 0x6300
	BlkIOPortSize  = 0x100
)

type Blk struct {
	Hdr blkHdr

	VirtQueue    [1]*VirtQueue
	Mem          []byte
	LastAvailIdx [1]uint16

	kick chan interface{}

	irq         uint8
	IRQInjector IRQInjector
}

type blkHdr struct {
	commonHeader commonHeader
	blkHeader    blkHeader
}

func (h blkHdr) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type blkHeader struct {
	capacity uint64
}

func (v Blk) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:    0x1001,
		VendorID:    0x1AF4,
		HeaderType:  0,
		SubsystemID: 2, // Block Device
		Command:     1, // Enable IO port
		BAR: [6]uint32{
			BlkIOPortStart | 0x1,
		},
		// https://github.com/torvalds/linux/blob/fb3b0673b7d5b477ed104949450cd511337ba3c6/drivers/pci/setup-irq.c#L30-L55
		InterruptPin: 1,
		// https://www.webopedia.com/reference/irqnumbers/
		InterruptLine: v.irq,
	}
}

func (v Blk) IOInHandler(port uint64, bytes []byte) error {
	offset := int(port - BlkIOPortStart)

	b, err := v.Hdr.Bytes()
	if err != nil {
		return err
	}

	l := len(bytes)
	copy(bytes[:l], b[offset:offset+l])

	return nil
}

func (v *Blk) IOThreadEntry() {
	for range v.kick {
		for v.IO() == nil {
		}
	}
}

func (v *Blk) IO() error {
	fmt.Printf("no io blk")
	return errors.New("no io")
}

func (v *Blk) IOOutHandler(port uint64, bytes []byte) error {
	offset := int(port - BlkIOPortStart)

	switch offset {
	case 8:
		fmt.Printf("pfn written!\r\n")
		// Queue PFN is aligned to page (4096 bytes)
		physAddr := uint32(pci.BytesToNum(bytes) * 4096)
		v.VirtQueue[v.Hdr.commonHeader.queueSEL] = (*VirtQueue)(unsafe.Pointer(&v.Mem[physAddr]))
	case 14:
		fmt.Printf("sel written!\r\n")
		v.Hdr.commonHeader.queueSEL = uint16(pci.BytesToNum(bytes))
	case 16:
		fmt.Printf("kick written!\r\n")
		v.Hdr.commonHeader.isr = 0x0
		v.kick <- true
	case 19:
		fmt.Printf("ISR was written!\r\n")
	default:
	}

	return nil
}

func (v Blk) GetIORange() (start, end uint64) {
	return BlkIOPortStart, BlkIOPortStart + BlkIOPortSize
}

func NewBlk(irq uint8, irqInjector IRQInjector, mem []byte) *Blk {
	res := &Blk{
		Hdr: blkHdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
				isr:      0x0,
			},
			blkHeader: blkHeader {
				capacity: 0x100,
			},
		},
		irq:          irq,
		IRQInjector:  irqInjector,
		kick:         make(chan interface{}),
		Mem:          mem,
		VirtQueue:    [1]*VirtQueue{},
		LastAvailIdx: [1]uint16{0},
	}

	return res
}
