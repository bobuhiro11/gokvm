package virtio_test

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/bobuhiro11/gokvm/virtio"
)

func TestGetDeviceHeader(t *testing.T) {
	t.Parallel()

	v := virtio.NewNet([]byte{})
	expected := uint16(0x1000)
	actual := v.GetDeviceHeader().DeviceID

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestGetIORange(t *testing.T) {
	t.Parallel()

	expected := uint64(virtio.IOPortSize)
	s, e := virtio.NewNet([]byte{}).GetIORange()
	actual := e - s

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestIOInHandler(t *testing.T) {
	t.Parallel()

	expected := []byte{0x08, 0x00}
	v := virtio.NewNet([]byte{})
	actual := make([]byte, 2)
	_ = v.IOInHandler(virtio.IOPortStart+12, actual)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestSetQueuePhysAddr(t *testing.T) {
	t.Parallel()

	mem := make([]byte, 0x1000000)
	v := virtio.NewNet(mem)
	base := uint32(uintptr(unsafe.Pointer(&(v.(*virtio.Net).Mem[0]))))

	expected := [2]uint32{
		base + 0x00345000,
		base + 0x0089a000,
	}

	_ = v.IOOutHandler(virtio.IOPortStart+14, []byte{0x0, 0x0})              // Select Queue #0
	_ = v.IOOutHandler(virtio.IOPortStart+8, []byte{0x45, 0x03, 0x00, 0x00}) // Set Phys Address

	_ = v.IOOutHandler(virtio.IOPortStart+14, []byte{0x1, 0x0})              // Select Queue #1
	_ = v.IOOutHandler(virtio.IOPortStart+8, []byte{0x9a, 0x08, 0x00, 0x00}) // Set Phys Address

	actual := [2]uint32{
		uint32(uintptr(unsafe.Pointer(v.(*virtio.Net).VirtQueue[0]))),
		uint32(uintptr(unsafe.Pointer(v.(*virtio.Net).VirtQueue[1]))),
	}

	for i := 0; i < 2; i++ {
		if expected[0] != actual[0] {
			t.Fatalf("expected[%d]: 0x%x, actual[%d]: 0x%x\n", i, expected[i], i, actual[i])
		}
	}
}
