package device

import "errors"

var errDataLenInvalid = errors.New("invalid data size on port")

// IODevice describes the interface a IO-Port device must implement regardless of the
// bus it is attached to.
// Clean up and unifying pci.Device and IODevice of this package will be required.
type IODevice interface {
	Read(uint64, []byte) error
	Write(uint64, []byte) error
	IOPort() uint64
	Size() uint64
}
