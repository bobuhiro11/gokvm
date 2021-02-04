package kvm

import (
	"io/ioutil"
	"os"
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

	numInterrupts = 0x100
)

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
	_                          [7]uint8
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

func CreateVCPU(vmFd uintptr) (uintptr, error) {
	return ioctl(vmFd, uintptr(kvmCreateVCPU), uintptr(0))
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

const (
	memSize = 1 << 30

	// bootParamAddr     = 0x10000.
	// cmdlineAddr       = 0x20000.

	kernelAddr = 0x100000
	initrdAddr = 0xf000000
)

type LinuxGuest struct {
	kvmFd, vmFd, vcpuFd uintptr
	mem                 []byte
}

func NewLinuxGuest(bzImagePath, initPath string) (*LinuxGuest, error) {
	g := &LinuxGuest{}
	devKVM, _ := os.OpenFile("/dev/kvm", os.O_RDWR, 0644)
	g.kvmFd = devKVM.Fd()
	g.vmFd, _ = CreateVM(g.kvmFd)
	g.vcpuFd, _ = CreateVCPU(g.vmFd)
	g.mem, _ = syscall.Mmap(-1, 0, memSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)

	// Load kernel
	bzImage, err := ioutil.ReadFile(bzImagePath)
	if err != nil {
		return g, err
	}

	offset := 0 // copy to g.mem with offest setupsz

	for i := 0; i < len(bzImage); i++ {
		g.mem[kernelAddr+i+offset] = bzImage[i]
	}

	// Load initrd
	initrd, err := ioutil.ReadFile(initPath)
	if err != nil {
		return g, err
	}

	for i := 0; i < len(initrd); i++ {
		g.mem[initrdAddr+i] = initrd[i]
	}

	if err = g.initRegs(); err != nil {
		return g, err
	}

	if err = g.initSregs(); err != nil {
		return g, err
	}

	return g, nil
}

func (g *LinuxGuest) initRegs() error {
	regs, _ := GetRegs(g.vcpuFd)
	regs.RFLAGS = 2
	regs.RIP = 0x100000
	regs.RSI = 0x10000

	if err := SetRegs(g.vcpuFd, regs); err != nil {
		return err
	}

	return nil
}

func (g *LinuxGuest) initSregs() error {
	sregs, _ := GetSregs(g.vcpuFd)

	// set all segment flat
	sregs.CS.Base, sregs.CS.Limit, sregs.CS.G = 0, 0xFFFFFFFF, 1
	sregs.DS.Base, sregs.DS.Limit, sregs.DS.G = 0, 0xFFFFFFFF, 1
	sregs.FS.Base, sregs.FS.Limit, sregs.FS.G = 0, 0xFFFFFFFF, 1
	sregs.GS.Base, sregs.GS.Limit, sregs.GS.G = 0, 0xFFFFFFFF, 1
	sregs.ES.Base, sregs.ES.Limit, sregs.ES.G = 0, 0xFFFFFFFF, 1
	sregs.SS.Base, sregs.SS.Limit, sregs.SS.G = 0, 0xFFFFFFFF, 1

	sregs.CS.DB, sregs.SS.DB = 1, 1
	sregs.CR0 |= 1 // protected mode

	if err := SetSregs(g.vcpuFd, sregs); err != nil {
		return err
	}

	return nil
}

func (g *LinuxGuest) Run(ioportHandler func(port uint32, isIn bool, value byte)) error {
	return nil
}
