package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unsafe"

	"github.com/bobuhiro11/gokvm/pci"
)

var (
	ErrInvalidSel  = errors.New("queue sel is invalid")
	ErrIONotPermit = errors.New("IO is not permitted for virtio device")
	ErrNoTxPacket  = errors.New("no packet for tx")
)

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

func (v *Net) TxThreadEntry() {
	for range v.txKick {
		for v.Tx() == nil {
		}
	}
}

func (v *Net) Tx() error {
	sel := v.Hdr.commonHeader.queueSEL
	if sel == 0 {
		return ErrInvalidSel
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == availRing.Idx {
		return ErrNoTxPacket
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
	res := &Net{
		Hdr: Hdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
				isr:      0x0,
			},
		},
		irqCallback:  irqCallBack,
		txKick:       make(chan interface{}),
		tap:          tap,
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
