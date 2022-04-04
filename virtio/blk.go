package virtio

import (
	"bytes"
	"encoding/binary"
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

type blkReq struct {
	typ uint32
	_ uint32
	sector uint64
}

func (v *Blk) IO() error {
	sel := uint16(0)
	// v.dumpDesc(sel)
	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == availRing.Idx {
		return ErrNoTxPacket
	}

	for v.LastAvailIdx[sel] != availRing.Idx {
		descID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

		// This structure is holding both the index of the descriptor chain and the
		// number of bytes that were written to the memory as part of serving the request.
		usedRing.Ring[usedRing.Idx%QueueSize].Idx = uint32(descID)
		usedRing.Ring[usedRing.Idx%QueueSize].Len = 0

		var buf [3][]byte
		for i:=0; i<3; i++ {
			desc := v.VirtQueue[sel].DescTable[descID]
			buf[i] = make([]byte, desc.Len)
			copy(buf[i], v.Mem[desc.Addr:desc.Addr+uint64(desc.Len)])

			usedRing.Ring[usedRing.Idx%QueueSize].Len += desc.Len
			descID = desc.Next
		}

		// buf[0] contains type, reserved, and sector fields.
		// buf[1] contains raw io data.
		// buf[2] contains a status field.
		//
		// refs https://wiki.osdev.org/Virtio#Block_Device_Packets
		blkReq := *((*blkReq)(unsafe.Pointer(&buf[0][0])))
		data := buf[1]
		fmt.Printf("blkReq: %v, data len: %v\r\n", blkReq, len(data))

		usedRing.Idx++
		v.LastAvailIdx[sel]++
	}

	v.Hdr.commonHeader.isr = 0x1
	v.IRQInjector.InjectVirtioBlkIRQ()

	return nil
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

func (v Blk) dumpDesc(sel uint16) {
	fmt.Printf("[descriptor for queue%d]\r\n", sel)
	fmt.Printf("Addr       Len    Flags   Next Data\r\n")
	fmt.Printf("-----------------------------------\r\n")
	for j:=0; j<QueueSize; j++ {
		desc := v.VirtQueue[sel].DescTable[j]
		buf := make([]byte, desc.Len)
		copy(buf, v.Mem[desc.Addr: desc.Addr+uint64(desc.Len)])
		fmt.Printf("0x%08x 0x%04x 0x%05x %04d 0x%x\r\n",
		desc.Addr, desc.Len, desc.Flags, desc.Next, buf)
	}

	fmt.Printf("[avail ring for queue%d: flags=0x%x, idx=%d, used_event=%d]\r\n", sel,
	v.VirtQueue[sel].AvailRing.Flags,
	v.VirtQueue[sel].AvailRing.Idx,
	v.VirtQueue[sel].AvailRing.UsedEvent)
	fmt.Printf("Ring\r\n")
	fmt.Printf("----\r\n")
	for j:=0; j<QueueSize; j++ {
		fmt.Printf("%04d\r\n", v.VirtQueue[sel].AvailRing.Ring[j])
	}

	fmt.Printf("[used ring for queue%d: flags=0x%x, idx=%d, avail_event=%d]\r\n", sel,
	v.VirtQueue[sel].UsedRing.Flags,
	v.VirtQueue[sel].UsedRing.Idx,
	v.VirtQueue[sel].UsedRing.availEvent)
	fmt.Printf("DescID Len\r\n")
	fmt.Printf("----------\r\n")
	for j:=0; j<QueueSize; j++ {
		fmt.Printf("0x%04x 0x%1x\r\n",
		v.VirtQueue[sel].UsedRing.Ring[j].Idx,
		v.VirtQueue[sel].UsedRing.Ring[j].Len)
	}
}
