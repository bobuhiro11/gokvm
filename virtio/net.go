package virtio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/bobuhiro11/gokvm/pci"
)

var (
	ErrIONotPermit = errors.New("IO is not permitted for virtio device")
	ErrNoTxPacket  = errors.New("no packet for tx")
	ErrNoRxPacket  = errors.New("no packet for rx")
	ErrVQNotInit   = errors.New("vq not initialized")
	ErrNoRxBuf     = errors.New("no buffer found for rx")
)

const (
	NetIOPortStart = 0x6200
	NetIOPortSize  = 0x100
)

type netHdr struct {
	commonHeader commonHeader
	_            netHeader
}

type Net struct {
	Hdr netHdr

	VirtQueue    [2]*VirtQueue
	Mem          []byte
	LastAvailIdx [2]uint16

	tap io.ReadWriter

	txKick    chan interface{}
	rxKick    chan os.Signal
	done      chan struct{}
	closeOnce sync.Once

	irq         uint8
	IRQInjector IRQInjector
}

func (h netHdr) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

type netHeader struct {
	_ [6]uint8 // mac
	_ uint16   // netStatus
	_ uint16   // maxVirtQueuePairs
}

func (v *Net) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:    0x1000,
		VendorID:    0x1AF4,
		HeaderType:  0,
		SubsystemID: 1, // Network Card
		Command:     1, // Enable IO port
		BAR: [6]uint32{
			NetIOPortStart | 0x1,
		},
		// https://github.com/torvalds/linux/blob/fb3b0673b7d5b477ed104949450cd511337ba3c6/drivers/pci/setup-irq.c#L30-L55
		InterruptPin: 1,
		// https://www.webopedia.com/reference/irqnumbers/
		InterruptLine: v.irq,
	}
}

func (v *Net) Read(port uint64, bytes []byte) error {
	offset := int(port - NetIOPortStart)

	if int(v.Hdr.commonHeader.queueSEL) >= len(v.VirtQueue) {
		v.Hdr.commonHeader.queueNUM = 0
	} else {
		v.Hdr.commonHeader.queueNUM = QueueSize
	}

	b, err := v.Hdr.Bytes()
	if err != nil {
		return err
	}

	l := len(bytes)
	copy(bytes[:l], b[offset:offset+l])

	// ISR is at offset 19 in the virtio common header.
	// Per the virtio spec, reading ISR clears it.
	if offset <= 19 && offset+l > 19 {
		v.Hdr.commonHeader.isr = 0
	}

	return nil
}

func (v *Net) RxThreadEntry() {
	log.Println("virtio-net: RxThreadEntry started")

	for {
		select {
		case <-v.done:
			log.Println("virtio-net: RxThreadEntry " +
				"received done signal")

			return
		case <-v.rxKick:
			for v.Rx() == nil {
			}
		}
	}
}

func (v *Net) Rx() error {
	// read raw packet from tap device
	packet := make([]byte, 4096)

	n, err := v.tap.Read(packet)
	if err != nil {
		return ErrNoRxPacket
	}

	packet = packet[:n]

	// append struct virtio_net_hdr
	packet = append(make([]byte, 10), packet...)

	sel := 0

	if v.VirtQueue[sel] == nil {
		return ErrVQNotInit
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == availRing.Idx {
		return ErrNoRxBuf
	}

	const NONE = uint16(256)
	headDescID := NONE
	prevDescID := NONE

	for len(packet) > 0 {
		descID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

		// head of vring chain
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

		usedRing.Ring[usedRing.Idx%QueueSize].Len += l

		if prevDescID != NONE {
			v.VirtQueue[sel].DescTable[prevDescID].Flags |= 0x1
			v.VirtQueue[sel].DescTable[prevDescID].Next = descID
		}

		prevDescID = descID
		v.LastAvailIdx[sel]++
	}

	usedRing.Idx++

	v.Hdr.commonHeader.isr = 0x1

	return v.IRQInjector.InjectVirtioNetIRQ()
}

func (v *Net) TxThreadEntry() {
	log.Println("virtio-net: TxThreadEntry started")

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-v.done:
			log.Println("virtio-net: TxThreadEntry " +
				"received done signal")

			return
		case <-v.txKick:
			for v.Tx() == nil {
			}
		case <-ticker.C:
			for v.Tx() == nil {
			}

			if v.Hdr.commonHeader.isr != 0 {
				_ = v.IRQInjector.InjectVirtioNetIRQ()
			}
		}
	}
}

func (v *Net) Tx() error {
	const sel = 1

	if v.VirtQueue[sel] == nil {
		return ErrVQNotInit
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

	v.Hdr.commonHeader.isr = 0x1

	return v.IRQInjector.InjectVirtioNetIRQ()
}

func (v *Net) Write(port uint64, bytes []byte) error {
	offset := int(port - NetIOPortStart)

	switch offset {
	case 8:
		// Queue PFN is aligned to page (4096 bytes)
		sel := v.Hdr.commonHeader.queueSEL
		if int(sel) >= len(v.VirtQueue) {
			break
		}

		physAddr := uint32(pci.BytesToNum(bytes) * 4096)
		v.VirtQueue[sel] = (*VirtQueue)(unsafe.Pointer(&v.Mem[physAddr]))
	case 14:
		v.Hdr.commonHeader.queueSEL = uint16(pci.BytesToNum(bytes))
	case 16:
		queueIdx := pci.BytesToNum(bytes)
		switch queueIdx {
		case 0:
			// RX queue kick: silently drop.
			// RX is driven by SIGIO signals.
		case 1:
			// TX queue kick: non-blocking send.
			select {
			case v.txKick <- true:
			default:
			}
		default:
			log.Printf(
				"virtio-net: unexpected queue %d",
				queueIdx,
			)
		}
	case 19:
	default:
	}

	return nil
}

func (v *Net) IOPort() uint64 {
	return NetIOPortStart
}

func (v *Net) Size() uint64 {
	return NetIOPortSize
}

func (v *Net) Close() error {
	log.Println("virtio-net: Close called")
	signal.Stop(v.rxKick)

	v.closeOnce.Do(func() { close(v.done) })

	if c, ok := v.tap.(io.Closer); ok {
		return c.Close()
	}

	return nil
}

func NewNet(irq uint8, irqInjector IRQInjector, tap io.ReadWriter, mem []byte) *Net {
	res := &Net{
		Hdr: netHdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
				isr:      0x0,
			},
		},
		irq:          irq,
		IRQInjector:  irqInjector,
		txKick:       make(chan interface{}, 1),
		rxKick:       make(chan os.Signal, 1),
		done:         make(chan struct{}),
		tap:          tap,
		Mem:          mem,
		VirtQueue:    [2]*VirtQueue{},
		LastAvailIdx: [2]uint16{0, 0},
	}

	signal.Notify(res.rxKick, syscall.SIGIO)

	return res
}
