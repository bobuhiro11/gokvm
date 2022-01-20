package virtio

import (
	"errors"

	"github.com/bobuhiro11/gokvm/pci"
)

var ErrIONotPermit = errors.New("IO is not permitted for virtio device")

type Net struct{}

func (v Net) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:   0x1000,
		VendorID:   0x1AF4,
		HeaderType: 0,
	}
}

func (v Net) IOInHandler(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (v Net) IOOutHandler(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (v Net) GetIORange() (start, end uint64) {
	return 0, 0
}

func NewNet() pci.Device {
	return &Net{}
}
