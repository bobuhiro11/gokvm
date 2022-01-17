package virtio

import (
	"errors"

	"github.com/bobuhiro11/gokvm/pci"
)

var ErrIONotPermit = errors.New("IO is not permitted for virtio device")

type virtioNet struct{}

func (br virtioNet) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:   0x1000,
		VendorID:   0x1AF4,
		HeaderType: 0,
	}
}

func (br virtioNet) IOInHandler(port int, bytes []byte) error {
	return ErrIONotPermit
}

func (br virtioNet) IOOutHandler(port int, bytes []byte) error {
	return ErrIONotPermit
}

func (br virtioNet) GetIORange() (start int, end int) {
	return 0, 0
}

func NewVirtioNet() pci.Device {
	return &virtioNet{}
}
