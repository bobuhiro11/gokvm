package pci

import "errors"

var ErrIONotPermit = errors.New("IO is not permitted for PCI bridge")

type bridge struct{}

func (br bridge) GetDeviceHeader() DeviceHeader {
	return DeviceHeader{
		DeviceID:   0x6000,
		VendorID:   0x8086,
		HeaderType: 1,
	}
}

func (br bridge) IOInHandler(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (br bridge) IOOutHandler(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (br bridge) GetIORange() (start, end uint64) {
	return 0, 0
}

func NewBridge() Device {
	return &bridge{}
}
