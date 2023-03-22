package memory

import (
	"errors"
)

var errAddrSpaceOccupied = errors.New("address space occopied")

type AddressSpace struct {
	Name      string
	Start     uint64
	Size      uint32
	Addresses []*AddressSpace
}

func NewAddressSpace(name string, start uint64, size uint32) *AddressSpace {
	return &AddressSpace{
		Name:  name,
		Start: start,
		Size:  size,
	}
}

func (a *AddressSpace) AddAddress(addr *AddressSpace) error {
	if !a.IsFree(addr) {
		return errAddrSpaceOccupied
	}

	a.Addresses = append(a.Addresses, addr)

	return nil
}

func (a *AddressSpace) InRange(addr *AddressSpace) bool {
	return addr.Start+uint64(addr.Size) < a.Start+uint64(a.Size)
}

func (a *AddressSpace) IsFree(ad *AddressSpace) bool {
	for _, addr := range a.Addresses {
		if addr.InRange(addr) {
			return false
		}
	}

	return true
}
