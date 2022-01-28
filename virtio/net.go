package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/bobuhiro11/gokvm/pci"
)

var ErrIONotPermit = errors.New("IO is not permitted for virtio device")

const (
	IOPortStart = 0x6200
	IOPortSize  = 0x100

	QueueSize = 8
)

type Hdr struct {
	commonHeader commonHeader
	_            netHeader
}

type Net struct {
	Hdr Hdr

	QueuesPhysAddr [2]uint32
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
	_        uint8  // isr
}

type netHeader struct {
	_ [6]uint8 // mac
	_ uint16   // netStatus
	_ uint16   // maxVirtQueuePairs
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
		InterruptLine: 9,
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

func (v *Net) IOOutHandler(port uint64, bytes []byte) error {
	offset := int(port - IOPortStart)

	switch offset {
	case 8:
		// Queue PFN is aligned to page (4096 bytes)
		v.QueuesPhysAddr[v.Hdr.commonHeader.queueSEL] = uint32(pci.BytesToNum(bytes) * 4096)
	case 14:
		v.Hdr.commonHeader.queueSEL = uint16(pci.BytesToNum(bytes))
	case 16:
		fmt.Printf("Queue Notify was written!\r\n")
	case 19:
		fmt.Printf("ISR was written!\r\n")
	default:
	}

	return nil
}

func (v Net) GetIORange() (start, end uint64) {
	return IOPortStart, IOPortStart + IOPortSize
}

func NewNet() pci.Device {
	return &Net{
		Hdr: Hdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
			},
		},
		QueuesPhysAddr: [2]uint32{},
	}
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
