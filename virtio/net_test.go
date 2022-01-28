package virtio_test

import (
	"bytes"
	"testing"

	"github.com/bobuhiro11/gokvm/virtio"
)

func TestGetDeviceHeader(t *testing.T) {
	t.Parallel()

	v := virtio.NewNet()
	expected := uint16(0x1000)
	actual := v.GetDeviceHeader().DeviceID

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestGetIORange(t *testing.T) {
	t.Parallel()

	expected := uint64(virtio.IOPortSize)
	s, e := virtio.NewNet().GetIORange()
	actual := e - s

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestIOInHandler(t *testing.T) {
	t.Parallel()

	expected := []byte{0x08, 0x00}
	v := virtio.NewNet()
	actual := make([]byte, 2)
	_ = v.IOInHandler(virtio.IOPortStart+12, actual)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestSetQueuePhysAddr(t *testing.T) {
	t.Parallel()

	expected := [2]uint32{
		0x12345000,
		0x6789a000,
	}

	v := virtio.NewNet()

	_ = v.IOOutHandler(virtio.IOPortStart+14, []byte{0x0, 0x0})              // Select Queue #0
	_ = v.IOOutHandler(virtio.IOPortStart+8, []byte{0x45, 0x23, 0x01, 0x00}) // Set Phys Address

	_ = v.IOOutHandler(virtio.IOPortStart+14, []byte{0x1, 0x0})              // Select Queue #1
	_ = v.IOOutHandler(virtio.IOPortStart+8, []byte{0x9a, 0x78, 0x06, 0x00}) // Set Phys Address

	actual := v.(*virtio.Net).QueuesPhysAddr
	if expected != actual {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}
