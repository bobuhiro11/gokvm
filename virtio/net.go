package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	// "time"
	"unsafe"

	"github.com/bobuhiro11/gokvm/pci"
)

var ErrIONotPermit = errors.New("IO is not permitted for virtio device")

const (
	IOPortStart = 0x6200
	IOPortSize  = 0x100

	QueueSize = 32
)

type Hdr struct {
	commonHeader commonHeader
	_            netHeader
}

type Net struct {
	Hdr Hdr

	VirtQueue [2]*VirtQueue
	Mem       []byte
	LastAvailIdx [2]uint16

	// This callback is called when virtio request IRQ.
	irqCallback func(irq, level uint32)

	// This callback is called when virtio transmit packet.
	txCallBack func(packet []byte)
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
	// isr      uint8
	_      uint8
}

type netHeader struct {
	_ [6]uint8 // mac
	_ uint16   // netStatus
	_ uint16   // maxVirtQueuePairs
}

func (v *Net) InjectIRQ() {
	v.irqCallback(9, 0)
	v.irqCallback(9, 1)
	// v.Hdr.commonHeader.isr = 0x1
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

func (v *Net) IOInHandler(port uint64, bytes []byte) error {
	offset := int(port - IOPortStart)

	b, err := v.Hdr.Bytes()
	if err != nil {
		return err
	}

	l := len(bytes)
	copy(bytes[:l], b[offset:offset+l])

	// if offset == 19 {
		// disable ISR
		// v.Hdr.commonHeader.isr = 0x0
		// b, _ = v.Hdr.Bytes()
		// fmt.Printf("disable ISR %d\r\n", b[19])
	// }

	fmt.Printf("IOInHandler called. offset %d, bytes %v\r\n", offset, bytes)

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
		fmt.Printf("Queue Notify was written!\r\n")
		sel := v.Hdr.commonHeader.queueSEL
		v.dumpDesc(sel)
		for v.LastAvailIdx[sel] != v.VirtQueue[sel].AvailRing.Idx {
			buf := []byte{}
			descID := v.VirtQueue[sel].AvailRing.Ring[v.LastAvailIdx[sel]%QueueSize]
			v.VirtQueue[sel].UsedRing.Ring[v.VirtQueue[sel].UsedRing.Idx%QueueSize].Idx = uint32(descID)
			// This structure is holding both the index of the descriptor chain and the
			// number of bytes that were written to the memory as part of serving the request.
			v.VirtQueue[sel].UsedRing.Ring[v.VirtQueue[sel].UsedRing.Idx%QueueSize].Len = 0

			for {
				desc := v.VirtQueue[sel].DescTable[descID]
				if desc.Flags & 0x4 != 0 {
					fmt.Printf("Indirect descriptor is not suported yet")
				}
				if desc.Flags & 0x2 != 0 {
					fmt.Printf("Readonly descriptor is not suported yet")
				}
				b := make([]byte, desc.Len)
				copy(b, v.Mem[desc.Addr: desc.Addr+uint64(desc.Len)])
				buf = append(buf, b...)

				// The used ring is where the device returns buffers once
				// it is done with them: it is only written to by the device,
				// and read by the driver. Each entry in the ring is a pair:
				// id indicates the head entry of the descriptor chain describing
				// the buffer (this matches an entry placed in the available ring
				// by the guest earlier), and len the total of bytes written into
				// the buffer. 
				v.VirtQueue[sel].UsedRing.Ring[v.VirtQueue[sel].UsedRing.Idx%QueueSize].Len += desc.Len

				if desc.Flags & 0x1 != 0 {
					descID = desc.Next
				} else {
					break
				}
			}

			buf = buf[10:] // skip struct virtio_net_hdr
			fmt.Printf("packet data: 0x%x\r\n", buf)
			fmt.Printf("packet data: %#v\r\n", buf)
			v.txCallBack(buf)
			v.VirtQueue[sel].UsedRing.Idx++
			v.LastAvailIdx[sel]++
			v.dumpDesc(sel)
		}
		// const VIRTQ_AVAIL_F_NO_INTERRUPT = 1
		// if v.VirtQueue[sel].AvailRing.Flags & VIRTQ_AVAIL_F_NO_INTERRUPT == 0 {
			v.InjectIRQ()
		// }
	case 19:
		fmt.Printf("ISR was written!\r\n")
	default:
	}

	return nil
}

func (v Net) GetIORange() (start, end uint64) {
	return IOPortStart, IOPortStart + IOPortSize
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
	v.VirtQueue[sel].UsedRing.AvailEvent)
	fmt.Printf("DescID Len\r\n")
	fmt.Printf("----------\r\n")
	for j:=0; j<QueueSize; j++ {
		fmt.Printf("0x%04x 0x%1x\r\n",
		v.VirtQueue[sel].UsedRing.Ring[j].Idx,
		v.VirtQueue[sel].UsedRing.Ring[j].Len)
	}
}

func NewNet(irqCallBack func(irq, level uint32), txCallBack func (packet []byte), mem []byte) pci.Device {
	// const VIRTIO_NET_F_CTRL_VQ = 1<<17
	res := &Net{
		Hdr: Hdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
				// hostFeatures: VIRTIO_NET_F_CTRL_VQ,
				// isr: 0x0,
			},
		},
		irqCallback: irqCallBack,
		txCallBack: txCallBack,
		Mem:       mem,
		VirtQueue: [2]*VirtQueue{},
		
		// 最後に処理したAvailable Ring上のエントリの番号の次
		LastAvailIdx: [2]uint16{0, 0},
	}
	// go func() {
	// 	time.Sleep(10*time.Second)
	// 	for true {
	// 		time.Sleep(3*time.Second)
	// 		res.dumpDesc(1)
	// 		res.InjectIRQ()
	// 	}
	// }()
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
		AvailEvent uint16
	}
}
