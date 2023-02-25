package kvm

import (
	"unsafe"
)

type MSRList struct {
	NMSRs    uint32
	Indicies [100]uint32
}

// GetMSRIndexList returns the guest msrs that are supported.
// The list varies by kvm version and host processor, but does not change otherwise.
func GetMSRIndexList(kvmFd uintptr, list *MSRList) error {
	// This ugly hack is required to make the Ioctl work.
	// If tried like kvm.GetSupportedCPUID it doesn't work.
	// Maybe a difference in behavior on kernel side.
	tmp := struct {
		NMSRs uint32
	}{
		NMSRs: 100,
	}
	_, err := Ioctl(kvmFd,
		IIOWR(kvmGetMSRIndexList, unsafe.Sizeof(tmp)),
		uintptr(unsafe.Pointer(list)))

	return err
}
