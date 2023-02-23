package kvm

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	kvmGetAPIVersion     = 0x00
	kvmCreateVM          = 0x1
	kvmCheckExtension    = 0x03
	kvmGetVCPUMMapSize   = 0x04
	kvmGetSupportedCPUID = 0x05

	kvmGetEmulatedCPUID    = 0x09
	kvmCreateVCPU          = 0x41
	kvmGetDirtyLog         = 0x42
	kvmSetNrMMUPages       = 0x44
	kvmGetNrMMUPages       = 0x45
	kvmSetUserMemoryRegion = 0x46
	kvmSetTSSAddr          = 0x47
	kvmSetIdentityMapAddr  = 0x48

	kvmCreateIRQChip = 0x60
	kvmGetIRQChip    = 0x62
	kvmSetIRQChip    = 0x63
	kvmIRQLineStatus = 0x67

	kvmResgisterCoalescedMMIO   = 0x67
	kvmUnResgisterCoalescedMMIO = 0x68

	kvmSetGSIRouting = 0x6A

	kvmCreatePIT2 = 0x77
	kvmSetClock   = 0x7B
	kvmGetClock   = 0x7C

	kvmRun      = 0x80
	kvmGetRegs  = 0x81
	kvmSetRegs  = 0x82
	kvmGetSregs = 0x83
	kvmSetSregs = 0x84

	kvmSetCPUID2 = 0x90

	kvmGetPIT2 = 0x9F
	kvmSetPIT2 = 0xA0

	kvmSetTSCKHz = 0xA2
	kvmGetTSCKHz = 0xA3

	kvmCreateDev = 0xE0
)

// ExitType is a virtual machine exit type.
//
//go:generate stringer -type=ExitType
type ExitType uint

const (
	EXITUNKNOWN       ExitType = 0
	EXITEXCEPTION     ExitType = 1
	EXITIO            ExitType = 2
	EXITHYPERCALL     ExitType = 3
	EXITDEBUG         ExitType = 4
	EXITHLT           ExitType = 5
	EXITMMIO          ExitType = 6
	EXITIRQWINDOWOPEN ExitType = 7
	EXITSHUTDOWN      ExitType = 8
	EXITFAILENTRY     ExitType = 9
	EXITINTR          ExitType = 10
	EXITSETTPR        ExitType = 11
	EXITTPRACCESS     ExitType = 12
	EXITS390SIEIC     ExitType = 13
	EXITS390RESET     ExitType = 14
	EXITDCR           ExitType = 15
	EXITNMI           ExitType = 16
	EXITINTERNALERROR ExitType = 17

	EXITIOIN  = 0
	EXITIOOUT = 1
)

const (
	numInterrupts   = 0x100
	CPUIDFeatures   = 0x40000001
	CPUIDSignature  = 0x40000000
	CPUIDFuncPerMon = 0x0A
)

var (
	// ErrUnexpectedExitReason is any error that we do not understand.
	ErrUnexpectedExitReason = errors.New("unexpected kvm exit reason")

	// ErrDebug is a debug exit, caused by single step or breakpoint.
	ErrDebug = errors.New("debug exit")
)

// RunData defines the data used to run a VM.
type RunData struct {
	RequestInterruptWindow     uint8
	ImmediateExit              uint8
	_                          [6]uint8
	ExitReason                 uint32
	ReadyForInterruptInjection uint8
	IfFlag                     uint8
	_                          [2]uint8
	CR8                        uint64
	ApicBase                   uint64
	Data                       [32]uint64
}

// IO interprets IO requests from a VM, by unpacking RunData.Data[0:1].
func (r *RunData) IO() (uint64, uint64, uint64, uint64, uint64) {
	direction := r.Data[0] & 0xFF
	size := (r.Data[0] >> 8) & 0xFF
	port := (r.Data[0] >> 16) & 0xFFFF
	count := (r.Data[0] >> 32) & 0xFFFFFFFF
	offset := r.Data[1]

	return direction, size, port, count, offset
}

// GetAPIVersion gets the qemu API version, which changes rarely if at all.
func GetAPIVersion(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, IIO(kvmGetAPIVersion), uintptr(0))
}

// CreateVM creates a KVM from the KVM device fd, i.e. /dev/kvm.
func CreateVM(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, IIO(kvmCreateVM), uintptr(0))
}

// CreateVCPU creates a single virtual CPU from the virtual machine FD.
// Thus, the progression:
// fd from opening /dev/kvm
// vmfd from creating a vm from the fd
// vcpu fd from the vmfd.
func CreateVCPU(vmFd uintptr, vcpuID int) (uintptr, error) {
	return Ioctl(vmFd, IIO(kvmCreateVCPU), uintptr(vcpuID))
}

// Run runs a single vcpu from the vcpufd from createvcpu.
func Run(vcpuFd uintptr) error {
	_, err := Ioctl(vcpuFd, IIO(kvmRun), uintptr(0))
	if err != nil {
		// refs: https://github.com/kvmtool/kvmtool/blob/415f92c33a227c02f6719d4594af6fad10f07abf/kvm-cpu.c#L44
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
			return nil
		}
	}

	return err
}

// GetVCPUMmapSize returns the size of the VCPU region. This size is
// required for interacting with the vcpu, as the struct size can change
// over time.
func GetVCPUMMmapSize(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, IIO(kvmGetVCPUMMapSize), uintptr(0))
}

func SetTSCKHz(vcpuFd uintptr, freq uint64) error {
	_, err := Ioctl(vcpuFd,
		IIO(kvmSetTSCKHz), uintptr(freq))

	return err
}

func GetTSCKHz(vcpuFd uintptr) (uint64, error) {
	ret, err := Ioctl(vcpuFd,
		IIO(kvmGetTSCKHz), 0)
	if err != nil {
		return 0, err
	}

	return uint64(ret), nil
}

type ClockFlag uint32

const (
	TSCStable ClockFlag = 2
	Realtime  ClockFlag = (1 << 2)
	HostTSC   ClockFlag = (1 << 3)
)

type ClockData struct {
	Clock    uint64
	Flags    uint32
	_        uint32
	Realtime uint64
	HostTSC  uint64
	_        [4]uint32
}

// SetClock sets the current timestamp of kvmclock to the value specified in its parameter.
// In conjunction with KVM_GET_CLOCK, it is used to ensure monotonicity on scenarios such as migration.
func SetClock(vmFd uintptr, cd *ClockData) error {
	_, err := Ioctl(vmFd,
		IIOW(kvmSetClock, unsafe.Sizeof(ClockData{})),
		uintptr(unsafe.Pointer(cd)))

	return err
}

// GetClock gets the current timestamp of kvmclock as seen by the current guest.
// In conjunction with KVM_SET_CLOCK, it is used to ensure monotonicity on scenarios such as migration.
func GetClock(vmFd uintptr, cd *ClockData) error {
	_, err := Ioctl(vmFd,
		IIOR(kvmGetClock, unsafe.Sizeof(ClockData{})),
		uintptr(unsafe.Pointer(cd)))

	return err
}

type DevType uint32

const (
	DevFSLMPIC20 DevType = 1 + iota
	DevFSLMPIC42
	DevXICS
	DevVFIO
	_
	DevFLIC
	_
	_
	DevXIVE
	_
	DevMAX
)

type Device struct {
	Type  uint32
	Fd    uint32
	Flags uint32
}

// CreateDev creates an emulated device in the kernel.
// The file descriptor returned in fd can be used with KVM_SET/GET/HAS_DEVICE_ATTR.
func CreateDev(vmFd uintptr, dev *Device) error {
	_, err := Ioctl(vmFd,
		IIOWR(kvmCreateDev, unsafe.Sizeof(Device{})),
		uintptr(unsafe.Pointer(dev)))

	return err
}
