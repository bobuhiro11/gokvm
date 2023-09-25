package virtio_test

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/bobuhiro11/gokvm/virtio"
)

type mockInjector struct {
	called bool
}

func (m *mockInjector) InjectVirtioNetIRQ() error {
	m.called = true

	return nil
}

func (m *mockInjector) InjectVirtioBlkIRQ() error {
	m.called = true

	return nil
}

func TestNetGetDeviceHeader(t *testing.T) {
	t.Parallel()

	v := virtio.NewNet(9, &mockInjector{}, bytes.NewBuffer([]byte{}), []byte{})
	expected := uint16(0x1000)
	actual := v.GetDeviceHeader().DeviceID

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNetGetIORange(t *testing.T) {
	t.Parallel()

	expected := uint64(virtio.NetIOPortSize)
	actual := virtio.NewNet(9, &mockInjector{}, bytes.NewBuffer([]byte{}), []byte{}).Size()

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestNetIOInHandler(t *testing.T) {
	t.Parallel()

	expected := []byte{0x20, 0x00}
	v := virtio.NewNet(9, &mockInjector{}, bytes.NewBuffer([]byte{}), []byte{})
	actual := make([]byte, 2)
	_ = v.Read(virtio.NetIOPortStart+12, actual)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestSetQueuePhysAddr(t *testing.T) {
	t.Parallel()

	mem := make([]byte, 0x1000000)
	v := virtio.NewNet(9, &mockInjector{}, bytes.NewBuffer([]byte{}), mem)
	base := uint32(uintptr(unsafe.Pointer(&(v.Mem[0]))))

	expected := [2]uint32{
		base + 0x00345000,
		base + 0x0089a000,
	}

	_ = v.Write(virtio.NetIOPortStart+14, []byte{0x0, 0x0})              // Select Queue #0
	_ = v.Write(virtio.NetIOPortStart+8, []byte{0x45, 0x03, 0x00, 0x00}) // Set Phys Address

	_ = v.Write(virtio.NetIOPortStart+14, []byte{0x1, 0x0})              // Select Queue #1
	_ = v.Write(virtio.NetIOPortStart+8, []byte{0x9a, 0x08, 0x00, 0x00}) // Set Phys Address

	actual := [2]uint32{
		uint32(uintptr(unsafe.Pointer(v.VirtQueue[0]))),
		uint32(uintptr(unsafe.Pointer(v.VirtQueue[1]))),
	}

	for i := 0; i < 2; i++ {
		if expected[0] != actual[0] {
			t.Fatalf("expected[%d]: 0x%x, actual[%d]: 0x%x\n", i, expected[i], i, actual[i])
		}
	}
}

func TestQueueNotifyHandler(t *testing.T) {
	t.Parallel()

	expected := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	b := bytes.NewBuffer([]byte{})

	mem := make([]byte, 0x1000000)
	v := virtio.NewNet(9, &mockInjector{}, b, mem)

	// Size of struct virtio_net_hdr
	const K = 10

	copy(mem[0x100+K:0x100+K+2], []byte{0xaa, 0xbb})
	copy(mem[0x200:0x200+2], []byte{0xcc, 0xdd})

	// Select Queue #1
	sel := byte(1)
	_ = v.Write(virtio.NetIOPortStart+14, []byte{sel, 0x0})

	// Init virt queue
	vq := virtio.VirtQueue{}

	vq.DescTable[0].Addr = 0x100
	vq.DescTable[0].Len = K + 2
	vq.DescTable[0].Flags = 0x1
	vq.DescTable[0].Next = 0x1

	vq.DescTable[1].Addr = 0x200
	vq.DescTable[1].Len = 2

	vq.AvailRing.Idx = 1
	v.VirtQueue[sel] = &vq

	if err := v.Tx(); err != nil {
		t.Fatalf("err: %v\n", err)
	}

	if !v.IRQInjector.(*mockInjector).called {
		t.Fatalf("irqInjected = false\n")
	}

	if !bytes.Equal(expected, b.Bytes()) {
		t.Fatalf("expected: %v, actual: %v", expected, b.Bytes())
	}
}

func TestRx(t *testing.T) {
	t.Parallel()

	expected := []byte{0xaa, 0xbb}
	mem := make([]byte, 0x1000000)
	v := virtio.NewNet(9, &mockInjector{}, bytes.NewBuffer(expected), mem)

	// Init virt queue
	vq := virtio.VirtQueue{}
	vq.AvailRing.Idx = 1
	vq.DescTable[0].Addr = 0x100
	vq.DescTable[0].Len = 0x200
	v.VirtQueue[0] = &vq

	// Size of struct virtio_net_hdr
	const K = 10

	if err := v.Rx(); err != nil {
		t.Fatalf("err: %v\n", err)
	}

	if !v.IRQInjector.(*mockInjector).called {
		t.Fatalf("irqInjected = false\n")
	}

	actual := mem[0x100+K : 0x100+K+2]
	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}
