package virtio

import (
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/bobuhiro11/gokvm/migration"
	"github.com/bobuhiro11/gokvm/pci"
)

const (
	BlkIOPortStart = 0x6300
	BlkIOPortSize  = 0x100

	SectorSize = 512
)

// LoadU16 reads a uint16 through a non-inlined function
// call, preventing the compiler from caching the value
// across iterations. This is needed for shared memory
// fields (AvailRing.Idx, UsedRing.Idx) that are written
// by KVM vCPU threads via unsafe.Pointer.
//
//go:noinline
func LoadU16(p *uint16) uint16 { return *p }

// StoreAddU16 atomically-enough increments a uint16
// through a non-inlined function call, ensuring the
// write is visible to other threads.
//
//go:noinline
func StoreAddU16(p *uint16, delta uint16) {
	*p += delta
}

type Blk struct {
	file *os.File
	Hdr  blkHdr

	VirtQueue    [1]*VirtQueue
	Mem          []byte
	LastAvailIdx [1]uint16

	kick      chan interface{}
	done      chan struct{}
	closeOnce sync.Once
	threadWG  sync.WaitGroup // tracks IOThread goroutine

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

func (v *Blk) GetDeviceHeader() pci.DeviceHeader {
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

func (v *Blk) Read(port uint64, bytes []byte) error {
	offset := int(port - BlkIOPortStart)

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

func (v *Blk) IOThreadEntry() {
	v.threadWG.Add(1)
	defer v.threadWG.Done()

	log.Println("virtio-blk: IOThreadEntry started")

	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-v.done:
			log.Println("virtio-blk: IOThreadEntry " +
				"received done signal")

			return
		case <-v.kick:
			for v.IO() == nil {
			}

			if v.Hdr.commonHeader.isr != 0 {
				_ = v.IRQInjector.InjectVirtioBlkIRQ()
			}
		case <-ticker.C:
			for v.IO() == nil {
			}

			if v.Hdr.commonHeader.isr != 0 {
				_ = v.IRQInjector.InjectVirtioBlkIRQ()
			}
		}
	}
}

type BlkReq struct {
	Type   uint32
	_      uint32
	Sector uint64
}

func (v *Blk) IO() error {
	sel := uint16(0)

	if v.VirtQueue[sel] == nil {
		return ErrVQNotInit
	}

	availRing := &v.VirtQueue[sel].AvailRing
	usedRing := &v.VirtQueue[sel].UsedRing

	if v.LastAvailIdx[sel] == LoadU16(&availRing.Idx) {
		return ErrNoTxPacket
	}

	log.Printf("virtio-blk IO: avail=%d last=%d",
		LoadU16(&availRing.Idx), v.LastAvailIdx[sel])

	for v.LastAvailIdx[sel] != LoadU16(&availRing.Idx) {
		descID := availRing.Ring[v.LastAvailIdx[sel]%QueueSize]

		// This structure is holding both the index of
		// the descriptor chain and the number of bytes
		// written to memory as part of serving the
		// request.
		uidx := LoadU16(&usedRing.Idx)
		usedRing.Ring[uidx%QueueSize].Idx = uint32(descID)
		usedRing.Ring[uidx%QueueSize].Len = 0

		var buf [3][]byte

		for i := 0; i < 3; i++ {
			desc := v.VirtQueue[sel].DescTable[descID]
			buf[i] = v.Mem[desc.Addr : desc.Addr+uint64(desc.Len)]

			usedRing.Ring[uidx%QueueSize].Len += desc.Len
			descID = desc.Next
		}

		// buf[0] contains type, reserved, and sector.
		// buf[1] contains raw io data.
		// buf[2] contains a status field.
		//
		// refs https://wiki.osdev.org/Virtio#Block_Device_Packets
		blkReq := *((*BlkReq)(unsafe.Pointer(&buf[0][0])))
		data := buf[1]

		log.Printf("virtio-blk IO: type=%d sector=%d"+
			" len=%d", blkReq.Type, blkReq.Sector,
			len(data))

		var ioErr error

		if blkReq.Type&0x1 == 0x1 {
			_, ioErr = v.file.WriteAt(
				data,
				int64(blkReq.Sector*SectorSize),
			)
		} else {
			_, ioErr = v.file.ReadAt(
				data,
				int64(blkReq.Sector*SectorSize),
			)

			if ioErr == nil {
				ioErr = v.file.Sync()
			}
		}

		// Write status byte per virtio spec.
		if ioErr != nil {
			buf[2][0] = 1 // VIRTIO_BLK_S_IOERR
		} else {
			buf[2][0] = 0 // VIRTIO_BLK_S_OK
		}

		StoreAddU16(&usedRing.Idx, 1)
		v.LastAvailIdx[sel]++
	}

	v.Hdr.commonHeader.isr = 0x1
	if err := v.IRQInjector.InjectVirtioBlkIRQ(); err != nil {
		return err
	}

	return nil
}

func (v *Blk) Write(port uint64, bytes []byte) error {
	offset := int(port - BlkIOPortStart)

	switch offset {
	case 8:
		// Queue PFN is aligned to page (4096 bytes)
		sel := v.Hdr.commonHeader.queueSEL
		if int(sel) >= len(v.VirtQueue) {
			break
		}

		physAddr := uint32(pci.BytesToNum(bytes) * 4096)
		v.VirtQueue[sel] = (*VirtQueue)(
			unsafe.Pointer(&v.Mem[physAddr]))

		log.Printf("virtio-blk: queue %d PFN set,"+
			" physAddr=0x%x", sel, physAddr)
	case 14:
		v.Hdr.commonHeader.queueSEL = uint16(pci.BytesToNum(bytes))
	case 16:
		select {
		case v.kick <- true:
			log.Println("virtio-blk: kick sent")
		default:
			if v.VirtQueue[0] != nil {
				log.Printf("virtio-blk: kick dropped"+
					" (avail=%d last=%d)",
					LoadU16(
						&v.VirtQueue[0].AvailRing.Idx),
					v.LastAvailIdx[0])
			}
		}
	case 19:
	default:
	}

	return nil
}

func (v *Blk) IOPort() uint64 {
	return BlkIOPortStart
}

func (v *Blk) Size() uint64 {
	return BlkIOPortSize
}

func (v *Blk) Close() error {
	log.Println("virtio-blk: Close called")
	v.closeOnce.Do(func() { close(v.done) })

	return v.file.Close()
}

// WaitStopped blocks until the IOThread has exited.
// Call after Close() to ensure the thread is no longer writing to guest memory.
func (v *Blk) WaitStopped() { v.threadWG.Wait() }

// GetState returns the host-side state of the virtio-blk device.
// The caller must ensure the I/O thread is not running concurrently.
func (v *Blk) GetState() *migration.BlkState {
	s := &migration.BlkState{}

	// Capture header as raw bytes (preserves blank-identifier padding fields).
	hdrBytes := make([]byte, unsafe.Sizeof(v.Hdr))
	copy(hdrBytes, unsafe.Slice((*byte)(unsafe.Pointer(&v.Hdr)), unsafe.Sizeof(v.Hdr)))
	s.HdrBytes = hdrBytes

	s.LastAvailIdx = v.LastAvailIdx

	// Record the guest physical address of each initialised virtqueue.
	for i, vq := range v.VirtQueue {
		if vq != nil {
			s.QueuePhysAddr[i] = uint64(uintptr(unsafe.Pointer(vq)) - uintptr(unsafe.Pointer(&v.Mem[0])))
		}
	}

	return s
}

// SetState restores the host-side state of the virtio-blk device.
// mem must be the fully restored guest memory for this machine.
// The caller must ensure the I/O thread is not running concurrently.
func (v *Blk) SetState(s *migration.BlkState, mem []byte) {
	if len(s.HdrBytes) > 0 {
		sz := int(unsafe.Sizeof(v.Hdr))
		if len(s.HdrBytes) >= sz {
			copy(unsafe.Slice((*byte)(unsafe.Pointer(&v.Hdr)), sz), s.HdrBytes[:sz])
		}
	}

	v.Mem = mem
	v.LastAvailIdx = s.LastAvailIdx

	for i, pa := range s.QueuePhysAddr {
		if pa != 0 {
			v.VirtQueue[i] = (*VirtQueue)(unsafe.Pointer(&mem[pa]))
		}
	}
}

func NewBlk(path string, irq uint8, irqInjector IRQInjector, mem []byte) (*Blk, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	fileSize := uint64(fileInfo.Size())

	res := &Blk{
		Hdr: blkHdr{
			commonHeader: commonHeader{
				queueNUM: QueueSize,
				isr:      0x0,
			},
			blkHeader: blkHeader{
				capacity: fileSize / SectorSize,
			},
		},
		file:         file,
		irq:          irq,
		IRQInjector:  irqInjector,
		kick:         make(chan interface{}, QueueSize),
		done:         make(chan struct{}),
		Mem:          mem,
		VirtQueue:    [1]*VirtQueue{},
		LastAvailIdx: [1]uint16{0},
	}

	return res, nil
}
