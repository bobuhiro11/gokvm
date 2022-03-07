package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/pci"
)

var ErrIONotPermit = errors.New("IO is not permitted for virtio device")

const (
	IOPortStart = 0x6200
	IOPortSize  = 0x100

	// The number of free descriptors in virt queue must exceed
	// MAX_SKB_FRAGS (16). Otherwise, packet transmission from
	// the guest to the host will be stopped.
	//
	// refs https://github.com/torvalds/linux/blob/5859a2b/drivers/net/virtio_net.c#L1754
	QueueSize = 32

	interruptLine = 9
)

type Hdr struct {
	commonHeader commonHeader
	_            netHeader
}

type Net struct {
	Hdr Hdr

	VirtQueue    [2]*VirtQueue
	Mem          []byte
	LastAvailIdx [2]uint16

	tap io.ReadWriter

	rxKick <-chan os.Signal
	txKick chan interface{}

	// This callback is called when virtio request IRQ.
	irqCallback func(irq, level uint32)
}

func (h Hdr) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type commonHeader struct {
	_        uint32 // hostFeatures
	_        uint32 // guestFeatures
	_        uint32 // queuePFN
	queueNUM uint16
	queueSEL uint16
	_        uint16 // queueNotify
	_        uint8  // status
	isr      uint8
}

type netHeader struct {
	_ [6]uint8 // mac
	_ uint16   // netStatus
	_ uint16   // maxVirtQueuePairs
}

func (v *Net) InjectIRQ() {
	v.Hdr.commonHeader.isr = 0x1
	v.irqCallback(interruptLine, 0)
	v.irqCallback(interruptLine, 1)
}

func (v Net) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:    0x1000,
		VendorID:    0x1AF4,
		HeaderType:  0,
		SubsystemID: 1, // Network Card
		Command:     1, // Enable IO port
		BAR: [6]uint32{
			IOPortStart | 0x1,
		},
		// https://github.com/torvalds/linux/blob/fb3b0673b7d5b477ed104949450cd511337ba3c6/drivers/pci/setup-irq.c#L30-L55
		InterruptPin: 1,
		// https://www.webopedia.com/reference/irqnumbers/
		InterruptLine: interruptLine,
	}
}

func (v Net) IOInHandler(port uint64, bytes []byte) error {
	offset := int(port - IOPortStart)

	b, err := v.Hdr.Bytes()
	if err != nil {
		return err
	}

	l := len(bytes)
	copy(bytes[:l], b[offset:offset+l])

	return nil
}

func (v *Net) Rx() error {
	packet := make([]byte, 4096)
	n, err := v.tap.Read(packet)
	if err != nil {
		return fmt.Errorf("packet not found in tap\r\n")
	}
	packet = packet[:n]

	sel := 0
	// v.dumpDesc(0)

	if v.VirtQueue[sel] == nil {
		return fmt.Errorf("vq not initialized for rx\r\n")
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == availRing.Idx {
		return fmt.Errorf("no buffer found for rx\r\n")
	}

	// Append struct virtio_net_hdr
	packet = append(make([]byte,10), packet...)

	const NONE = uint16(256)
	headDescID := NONE
	prevDescID := NONE

	for len(packet) > 0 { // for chain
		descID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

		// decide head of vring chain for rx
		if headDescID == NONE {
			headDescID = descID

			// This structure is holding both the index of the descriptor chain and the
			// number of bytes that were written to the memory as part of serving the request.
			usedRing.Ring[usedRing.Idx%QueueSize].Idx = uint32(headDescID)
			usedRing.Ring[usedRing.Idx%QueueSize].Len = 0
		}

		desc := &v.VirtQueue[sel].DescTable[descID]
		l := uint32(len(packet))
		if l > desc.Len {
			l = desc.Len
		}

		copy(v.Mem[desc.Addr:desc.Addr+uint64(l)], packet[:l])
		packet = packet[l:]
		desc.Len = l
		// fmt.Printf("write packet to desc[%d], desc.Len = %d packet=%#v\r\n", descID, desc.Len, packet)

		usedRing.Ring[usedRing.Idx%QueueSize].Len += l

		if prevDescID != NONE {
			v.VirtQueue[sel].DescTable[prevDescID].Flags |= 0x1
			v.VirtQueue[sel].DescTable[prevDescID].Next = descID
		}
		prevDescID = descID
		v.LastAvailIdx[sel]++
	}
	usedRing.Idx++
	// v.Hdr.commonHeader.queueSEL = uint16(sel)
	v.InjectIRQ()

	return nil
}

func (v *Net) RxThreadEntry() {
	for _ = range v.rxKick {
		for v.Rx() == nil {
		}
	}
}

func (v *Net) TxThreadEntry() {
	for _ = range v.txKick {
		for v.Tx() == nil {
		}
	}
}

func (v *Net) Tx() error {
	sel := v.Hdr.commonHeader.queueSEL
	if sel == 0 {
		return fmt.Errorf("queue sel is invalid")
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == availRing.Idx {
		return fmt.Errorf("no packet for tx")
	}

	for v.LastAvailIdx[sel] != availRing.Idx {
		buf := []byte{}
		descID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

		// This structure is holding both the index of the descriptor chain and the
		// number of bytes that were written to the memory as part of serving the request.
		usedRing.Ring[usedRing.Idx%QueueSize].Idx = uint32(descID)
		usedRing.Ring[usedRing.Idx%QueueSize].Len = 0

		for {
			desc := v.VirtQueue[sel].DescTable[descID]

			b := make([]byte, desc.Len)
			copy(b, v.Mem[desc.Addr:desc.Addr+uint64(desc.Len)])
			buf = append(buf, b...)

			usedRing.Ring[usedRing.Idx%QueueSize].Len += desc.Len

			if desc.Flags&0x1 != 0 {
				descID = desc.Next
			} else {
				break
			}
		}

		// Skip struct virtio_net_hdr
		// refs https://github.com/torvalds/linux/blob/38f80f42/include/uapi/linux/virtio_net.h#L178-L191
		buf = buf[10:]

		if _, err := v.tap.Write(buf); err != nil {
			return err
		}
		usedRing.Idx++
		v.LastAvailIdx[sel]++
	}
	v.InjectIRQ()

	return nil
}

func (v *Net) IOOutHandler(port uint64, bytes []byte) error {
	offset := int(port - IOPortStart)

	switch offset {
	case 8:
		// Queue PFN is aligned to page (4096 bytes)
		physAddr := uint32(pci.BytesToNum(bytes) * 4096)
		v.VirtQueue[v.Hdr.commonHeader.queueSEL] = (*VirtQueue)(unsafe.Pointer(&v.Mem[physAddr]))
	case 14:
		v.Hdr.commonHeader.queueSEL = uint16(pci.BytesToNum(bytes))
	case 16:
		v.Hdr.commonHeader.isr = 0x0
		v.txKick <- true
	case 19:
		fmt.Printf("ISR was written!\r\n")
	default:
	}

	return nil
}

func (v Net) GetIORange() (start, end uint64) {
	return IOPortStart, IOPortStart + IOPortSize
}

func NewNet(irqCallBack func(irq, level uint32), tap io.ReadWriter, mem []byte) pci.Device {

	rxKick := make(chan os.Signal)
	txKick := make(chan interface{})

	signal.Notify(rxKick, syscall.SIGIO)

	res := &Net{
		Hdr: Hdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
				isr: 0x0,
			},
		},
		irqCallback: irqCallBack,
		rxKick: rxKick,
		txKick: txKick,
		tap: tap,
		Mem:          mem,
		VirtQueue:    [2]*VirtQueue{},
		LastAvailIdx: [2]uint16{0, 0},
	}

	return res
}

// refs: https://wiki.osdev.org/Virtio#Virtual_Queue_Descriptor
type VirtQueue struct {
	DescTable [QueueSize]struct {
		Addr  uint64
		Len   uint32
		Flags uint16
		Next  uint16
	}

	AvailRing struct {
		Flags     uint16
		Idx       uint16
		Ring      [QueueSize]uint16
		UsedEvent uint16
	}

	// padding for 4096 byte alignment
	_ [4096 - ((16*QueueSize + 6 + 2*QueueSize) % 4096)]uint8

	UsedRing struct {
		Flags uint16
		Idx   uint16
		Ring  [QueueSize]struct {
			Idx uint32
			Len uint32
		}
		availEvent uint16
	}
}

func (v Net) dumpDesc(sel uint16) {
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
