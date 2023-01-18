package kvm

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	kvmGetAPIVersion       = 44544
	kvmCreateVM            = 44545
	kvmCreateVCPU          = 44609
	kvmRun                 = 44672
	kvmGetVCPUMMapSize     = 44548
	kvmGetSregs            = 0x8138ae83
	kvmSetSregs            = 0x4138ae84
	kvmGetRegs             = 0x8090ae81
	kvmSetRegs             = 0x4090ae82
	kvmSetUserMemoryRegion = 1075883590
	kvmSetTSSAddr          = 0xae47
	kvmSetIdentityMapAddr  = 0x4008AE48
	kvmCreateIRQChip       = 0xAE60
	kvmCreatePIT2          = 0x4040AE77
	kvmGetSupportedCPUID   = 0xC008AE05
	kvmSetCPUID2           = 0x4008AE90
	kvmIRQLine             = 0xc008ae67
)

//go:generate stringer -type=ExitType
// ExitType is a virtual machine exit type.
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
	ErrUnexpectedEXITReason = errors.New("unexpected kvm exit reason")
	ErrDebug = errors.New("Debug Exit")
)

// Regs are registers for both 386 and amd64.
// In 386 mode, only some of them are used.
type Regs struct {
	RAX    uint64
	RBX    uint64
	RCX    uint64
	RDX    uint64
	RSI    uint64
	RDI    uint64
	RSP    uint64
	RBP    uint64
	R8     uint64
	R9     uint64
	R10    uint64
	R11    uint64
	R12    uint64
	R13    uint64
	R14    uint64
	R15    uint64
	RIP    uint64
	RFLAGS uint64
}

// Sregs are control registers, for memory mapping for the most part.
type Sregs struct {
	CS              Segment
	DS              Segment
	ES              Segment
	FS              Segment
	GS              Segment
	SS              Segment
	TR              Segment
	LDT             Segment
	GDT             Descriptor
	IDT             Descriptor
	CR0             uint64
	CR2             uint64
	CR3             uint64
	CR4             uint64
	CR8             uint64
	EFER            uint64
	ApicBase        uint64
	InterruptBitmap [(numInterrupts + 63) / 64]uint64
}

// Segment is an x86 segment descriptor.
type Segment struct {
	Base     uint64
	Limit    uint32
	Selector uint16
	Typ      uint8
	Present  uint8
	DPL      uint8
	DB       uint8
	S        uint8
	L        uint8
	G        uint8
	AVL      uint8
	Unusable uint8
	_        uint8
}

// Descriptor defines a GDT, LDT, or other pointer type.
type Descriptor struct {
	Base  uint64
	Limit uint16
	_     [3]uint16
}

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

// Ioctl is a convenience function to call ioctl.
// Its main purpose is to format arguments
// and return values to make things easier for
// programmers.
func Ioctl(fd, op, arg uintptr) (uintptr, error) {
	res, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, op, arg)
	if errno != 0 {
		return res, errno
	}

	return res, nil
}

// GetAPIVersion gets the qemu API version, which changes rarely if at all.
func GetAPIVersion(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, uintptr(kvmGetAPIVersion), uintptr(0))
}

// CreateVM creates a KVM from the KVM device fd, i.e. /dev/kvm.
func CreateVM(kvmFd uintptr) (uintptr, error) {
	return Ioctl(kvmFd, uintptr(kvmCreateVM), uintptr(0))
}

// DebugControl controls guest debug.
type DebugControl struct {
	Control  uint32
	_        uint32
	DebugReg [8]uint64
}

// SingleStep enables single stepping on all vCPUS in a VM. At present, it seems
// not to work. It is based on working code from the linuxboot Voodoo project.
func SingleStep(vmFd uintptr, onoff bool) error {
	const (
		// Enable enables debug options in the guest
		Enable = 1
		// SingleStep enables single step.
		SingleStep = 2
	)

	var (
		debug         [unsafe.Sizeof(DebugControl{})]byte
		setGuestDebug = IIOW(0x9b, unsafe.Sizeof(DebugControl{}))
	)

	if onoff {
		debug[2] = 0x0002 // 0000
		debug[0] = Enable | SingleStep
	}

	// this is not very nice, but it is easy.
	// And TBH, the tricks the Linux kernel people
	// play are a lot nastier.
	_, err := Ioctl(vmFd, setGuestDebug, uintptr(unsafe.Pointer(&debug[0])))

	return err
}

// CreateVCPU creates a single virtual CPU from the virtual machine FD.
// Thus, the progression:
// fd from opening /dev/kvm
// vmfd from creating a vm from the fd
// vcpu fd from the vmfd.
func CreateVCPU(vmFd uintptr, vcpuID int) (uintptr, error) {
	return Ioctl(vmFd, uintptr(kvmCreateVCPU), uintptr(vcpuID))
}

// Run runs a single vcpu from the vcpufd from createvcpu.
func Run(vcpuFd uintptr) error {
	_, err := Ioctl(vcpuFd, uintptr(kvmRun), uintptr(0))
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
	return Ioctl(kvmFd, uintptr(kvmGetVCPUMMapSize), uintptr(0))
}

// GetSRegs gets the special registers for a vcpu.
func GetSregs(vcpuFd uintptr) (Sregs, error) {
	sregs := Sregs{}
	_, err := Ioctl(vcpuFd, uintptr(kvmGetSregs), uintptr(unsafe.Pointer(&sregs)))

	return sregs, err
}

// SetSRegs sets the special registers for a vcpu.
func SetSregs(vcpuFd uintptr, sregs Sregs) error {
	_, err := Ioctl(vcpuFd, uintptr(kvmSetSregs), uintptr(unsafe.Pointer(&sregs)))

	return err
}

// GetRegs gets the general purpose registers for a vcpu.
func GetRegs(vcpuFd uintptr) (Regs, error) {
	regs := Regs{}
	_, err := Ioctl(vcpuFd, uintptr(kvmGetRegs), uintptr(unsafe.Pointer(&regs)))

	return regs, err
}

// SetRegs sets the general purpose registers for a vcpu.
func SetRegs(vcpuFd uintptr, regs Regs) error {
	_, err := Ioctl(vcpuFd, uintptr(kvmSetRegs), uintptr(unsafe.Pointer(&regs)))

	return err
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

// IRQLevel defines an IRQ as Level? Not sure.
type IRQLevel struct {
	IRQ   uint32
	Level uint32
}

// IRQLines sets the interrupt line for an IRQ.
func IRQLine(vmFd uintptr, irq, level uint32) error {
	irqLevel := IRQLevel{
		IRQ:   irq,
		Level: level,
	}

	_, err := Ioctl(vmFd, kvmIRQLine, uintptr(unsafe.Pointer(&irqLevel)))

	return err
}

// CreateIRQChip creates an IRQ device (chip) to which to attach interrupts?
func CreateIRQChip(vmFd uintptr) error {
	_, err := Ioctl(vmFd, kvmCreateIRQChip, 0)

	return err
}

// PitConfig defines properties of a programmable interrupt timer.
type PitConfig struct {
	Flags uint32
	_     [15]uint32
}

// CreatePIT2 creates a PIT type 2. Just having one was not enough.
func CreatePIT2(vmFd uintptr) error {
	pit := PitConfig{
		Flags: 0,
	}
	_, err := Ioctl(vmFd, kvmCreatePIT2, uintptr(unsafe.Pointer(&pit)))

	return err
}

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
	_, err := Ioctl(kvmFd, kvmGetSupportedCPUID, uintptr(unsafe.Pointer(kvmCPUID)))

	return err
}

// SetCPUID2 sets entries for a vCPU.
// The progression is, hence, get the CPUID entries for a vm, then set them into
// individual vCPUs. This seems odd, but in fact lets code tailor CPUID entries
// as needed.
func SetCPUID2(vcpuFd uintptr, kvmCPUID *CPUID) error {
	_, err := Ioctl(vcpuFd, kvmSetCPUID2, uintptr(unsafe.Pointer(kvmCPUID)))

	return err
}
