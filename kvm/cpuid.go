package kvm

import (
	"unsafe"
)

// CPUID is the set of CPUID entries returned by GetCPUID.
type CPUID struct {
	Nent    uint32
	Padding uint32
	Entries [100]CPUIDEntry2
}

// CPUIDEntry2 is one entry for CPUID. It took 2 tries to get it right :-)
// Thanks x86 :-).
type CPUIDEntry2 struct {
	Function uint32
	Index    uint32
	Flags    uint32
	Eax      uint32
	Ebx      uint32
	Ecx      uint32
	Edx      uint32
	Padding  [3]uint32
}

// GetSupportedCPUID gets all supported CPUID entries for a vm.
func GetSupportedCPUID(kvmFd uintptr, kvmCPUID *CPUID) error {
	_, err := Ioctl(kvmFd,
		IIOWR(kvmGetSupportedCPUID, unsafe.Sizeof(kvmCPUID)),
		uintptr(unsafe.Pointer(kvmCPUID)))

	return err
}

// SetCPUID2 sets entries for a vCPU.
// The progression is, hence, get the CPUID entries for a vm, then set them into
// individual vCPUs. This seems odd, but in fact lets code tailor CPUID entries
// as needed.
func SetCPUID2(vcpuFd uintptr, kvmCPUID *CPUID) error {
	_, err := Ioctl(vcpuFd,
		IIOW(kvmSetCPUID2, unsafe.Sizeof(kvmCPUID)),
		uintptr(unsafe.Pointer(kvmCPUID)))

	return err
}
