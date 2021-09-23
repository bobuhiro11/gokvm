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

	EXITUNKNOWN       = 0
	EXITEXCEPTION     = 1
	EXITIO            = 2
	EXITHYPERCALL     = 3
	EXITDEBUG         = 4
	EXITHLT           = 5
	EXITMMIO          = 6
	EXITIRQWINDOWOPEN = 7
	EXITSHUTDOWN      = 8
	EXITFAILENTRY     = 9
	EXITINTR          = 10
	EXITSETTPR        = 11
	EXITTPRACCESS     = 12
	EXITS390SIEIC     = 13
	EXITS390RESET     = 14
	EXITDCR           = 15
	EXITNMI           = 16
	EXITINTERNALERROR = 17

	EXITIOIN  = 0
	EXITIOOUT = 1

	numInterrupts  = 0x100
	CPUIDFeatures  = 0x40000001
	CPUIDSignature = 0x40000000
)

var ErrorUnexpectedEXITReason = errors.New("unexpected kvm exit reason")

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

type Descriptor struct {
	Base  uint64
	Limit uint16
	_     [3]uint16
}

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

func (r *RunData) IO() (uint64, uint64, uint64, uint64, uint64) {
	direction := r.Data[0] & 0xFF
	size := (r.Data[0] >> 8) & 0xFF
	port := (r.Data[0] >> 16) & 0xFFFF
	count := (r.Data[0] >> 32) & 0xFFFFFFFF
	offset := r.Data[1]

	return direction, size, port, count, offset
}

type UserspaceMemoryRegion struct {
	Slot          uint32
	Flags         uint32
	GuestPhysAddr uint64
	MemorySize    uint64
	UserspaceAddr uint64
}

func (r *UserspaceMemoryRegion) SetMemLogDirtyPages() {
	r.Flags |= 1 << 0
}

func (r *UserspaceMemoryRegion) SetMemReadonly() {
	r.Flags |= 1 << 1
}

func ioctl(fd, op, arg uintptr) (uintptr, error) {
	res, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, fd, op, arg)

	var err error = nil
	if errno != 0 {
		err = errno
	}

	return res, err
}

func GetAPIVersion(kvmFd uintptr) (uintptr, error) {
	return ioctl(kvmFd, uintptr(kvmGetAPIVersion), uintptr(0))
}

func CreateVM(kvmFd uintptr) (uintptr, error) {
	return ioctl(kvmFd, uintptr(kvmCreateVM), uintptr(0))
}

func CreateVCPU(vmFd uintptr, vcpuID int) (uintptr, error) {
	return ioctl(vmFd, uintptr(kvmCreateVCPU), uintptr(vcpuID))
}

func Run(vcpuFd uintptr) error {
	_, err := ioctl(vcpuFd, uintptr(kvmRun), uintptr(0))

	return err
}

func GetVCPUMMmapSize(kvmFd uintptr) (uintptr, error) {
	return ioctl(kvmFd, uintptr(kvmGetVCPUMMapSize), uintptr(0))
}

func GetSregs(vcpuFd uintptr) (Sregs, error) {
	sregs := Sregs{}
	_, err := ioctl(vcpuFd, uintptr(kvmGetSregs), uintptr(unsafe.Pointer(&sregs)))

	return sregs, err
}

func SetSregs(vcpuFd uintptr, sregs Sregs) error {
	_, err := ioctl(vcpuFd, uintptr(kvmSetSregs), uintptr(unsafe.Pointer(&sregs)))

	return err
}

func GetRegs(vcpuFd uintptr) (Regs, error) {
	regs := Regs{}
	_, err := ioctl(vcpuFd, uintptr(kvmGetRegs), uintptr(unsafe.Pointer(&regs)))

	return regs, err
}

func SetRegs(vcpuFd uintptr, regs Regs) error {
	_, err := ioctl(vcpuFd, uintptr(kvmSetRegs), uintptr(unsafe.Pointer(&regs)))

	return err
}

func SetUserMemoryRegion(vmFd uintptr, region *UserspaceMemoryRegion) error {
	_, err := ioctl(vmFd, uintptr(kvmSetUserMemoryRegion), uintptr(unsafe.Pointer(region)))

	return err
}

func SetTSSAddr(vmFd uintptr) error {
	_, err := ioctl(vmFd, kvmSetTSSAddr, 0xffffd000)

	return err
}

func SetIdentityMapAddr(vmFd uintptr) error {
	var mapAddr uint64 = 0xffffc000
	_, err := ioctl(vmFd, kvmSetIdentityMapAddr, uintptr(unsafe.Pointer(&mapAddr)))

	return err
}

type IRQLevel struct {
	IRQ   uint32
	Level uint32
}

func IRQLine(vmFd uintptr, irq, level uint32) error {
	irqLevel := IRQLevel{
		IRQ:   irq,
		Level: level,
	}

	_, err := ioctl(vmFd, kvmIRQLine, uintptr(unsafe.Pointer(&irqLevel)))

	return err
}

func CreateIRQChip(vmFd uintptr) error {
	_, err := ioctl(vmFd, kvmCreateIRQChip, 0)

	return err
}

type PitConfig struct {
	Flags uint32
	_     [15]uint32
}

func CreatePIT2(vmFd uintptr) error {
	pit := PitConfig{
		Flags: 0,
	}
	_, err := ioctl(vmFd, kvmCreatePIT2, uintptr(unsafe.Pointer(&pit)))

	return err
}

type CPUID struct {
	Nent    uint32
	Padding uint32
	Entries [100]CPUIDEntry2
}

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

func GetSupportedCPUID(kvmFd uintptr, kvmCPUID *CPUID) error {
	_, err := ioctl(kvmFd, kvmGetSupportedCPUID, uintptr(unsafe.Pointer(kvmCPUID)))

	return err
}

func SetCPUID2(vcpuFd uintptr, kvmCPUID *CPUID) error {
	_, err := ioctl(vcpuFd, kvmSetCPUID2, uintptr(unsafe.Pointer(kvmCPUID)))

	return err
}
