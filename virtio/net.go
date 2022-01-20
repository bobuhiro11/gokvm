package virtio

import (
	"errors"

	"github.com/bobuhiro11/gokvm/pci"
)

var ErrIONotPermit = errors.New("IO is not permitted for virtio device")

const (
	IOPortStart = 0x6200
	IOPortSize  = 0x100
)

type Net struct{}

func (v Net) GetDeviceHeader() pci.DeviceHeader {
	return pci.DeviceHeader{
		DeviceID:    0x1000,
		VendorID:    0x1AF4,
		HeaderType:  0,
		SubsystemID: 1, // Network Card
		Command:     1, // Enable IO port
		BAR: [6]uint32{
			IOPortStart | 0x1,
		},
		// https://github.com/torvalds/linux/blob/fb3b0673b7d5b477ed104949450cd511337ba3c6/drivers/pci/setup-irq.c#L30-L55
		InterruptPin: 1,
		// https://www.webopedia.com/reference/irqnumbers/
		InterruptLine: 9,
	}
}

func (v Net) IOInHandler(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (v Net) IOOutHandler(port uint64, bytes []byte) error {
	return ErrIONotPermit
}

func (v Net) GetIORange() (start, end uint64) {
	return IOPortStart, IOPortStart + IOPortSize
}

func NewNet() pci.Device {
	return &Net{}
}
