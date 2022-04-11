package virtio_test

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/bobuhiro11/gokvm/virtio"
)

func TestBlkGetDeviceHeader(t *testing.T) {
	t.Parallel()

	v, err := virtio.NewBlk("/dev/zero", 9, &mockInjector{}, []byte{})
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	expected := uint16(0x1001)
	actual := v.GetDeviceHeader().DeviceID

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestBlkGetIORange(t *testing.T) {
	t.Parallel()

	v, err := virtio.NewBlk("/dev/zero", 9, &mockInjector{}, []byte{})
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	s, e := v.GetIORange()
	actual := e - s
	expected := uint64(virtio.BlkIOPortSize)

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestBlkIOInHandler(t *testing.T) {
	t.Parallel()

	v, err := virtio.NewBlk("/dev/zero", 9, &mockInjector{}, []byte{})
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	expected := []byte{0x20, 0x00}
	actual := make([]byte, 2)
	_ = v.IOInHandler(virtio.BlkIOPortStart+12, actual)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestIO(t *testing.T) {
	t.Parallel()

	mem := make([]byte, 0x1000000)

	v, err := virtio.NewBlk("../vda.img", 10, &mockInjector{}, mem)
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	// Init virt queue
	vq := virtio.VirtQueue{}
	vq.AvailRing.Idx = 1

	// for blk request
	vq.DescTable[0].Addr = 0
	vq.DescTable[0].Len = 1
	vq.DescTable[0].Next = 1

	blkReq := (*virtio.BlkReq)(unsafe.Pointer(&mem[0]))
	blkReq.Type = 0
	blkReq.Sector = 2

	// for data
	vq.DescTable[1].Addr = 0x400
	vq.DescTable[1].Len = 0x200
	vq.DescTable[1].Next = 2

	v.VirtQueue[0] = &vq

	if err := v.IO(); err != nil {
		t.Fatalf("err: %v\n", err)
	}

	if !v.IRQInjector.(*mockInjector).called {
		t.Fatalf("irqInjected = false\n")
	}

	expected := []byte{0x53, 0xef}
	actual := mem[0x438:0x43a]

	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}
