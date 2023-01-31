package kvm

import "unsafe"

// UserSpaceMemoryRegion defines Memory Regions.
type UserspaceMemoryRegion struct {
	Slot          uint32
	Flags         uint32
	GuestPhysAddr uint64
	MemorySize    uint64
	UserspaceAddr uint64
}

// SetMemLogDirtyPages sets region flags to log dirty pages.
// This is useful in many situations, including migration.
func (r *UserspaceMemoryRegion) SetMemLogDirtyPages() {
	r.Flags |= 1 << 0
}

// SetMemReadonly marks a region as read only.
func (r *UserspaceMemoryRegion) SetMemReadonly() {
	r.Flags |= 1 << 1
}

// SetUserMemoryRegion adds a memory region to a vm -- not a vcpu, a vm.
func SetUserMemoryRegion(vmFd uintptr, region *UserspaceMemoryRegion) error {
	_, err := Ioctl(vmFd, uintptr(kvmSetUserMemoryRegion), uintptr(unsafe.Pointer(region)))

	return err
}

// SetTSSAddr sets the Task Segment Selector for a vm.
func SetTSSAddr(vmFd uintptr) error {
	_, err := Ioctl(vmFd, kvmSetTSSAddr, 0xffffd000)

	return err
}

// SetIdentityMapAddr sets the address of a 4k-sized-page for a vm.
func SetIdentityMapAddr(vmFd uintptr) error {
	var mapAddr uint64 = 0xffffc000
	_, err := Ioctl(vmFd, kvmSetIdentityMapAddr, uintptr(unsafe.Pointer(&mapAddr)))

	return err
}
