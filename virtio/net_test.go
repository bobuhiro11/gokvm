package virtio_test

import (
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
