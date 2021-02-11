package kvm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
	"unsafe"

	"github.com/nmi/gokvm/bootproto"
	"github.com/nmi/gokvm/serial"
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
	// fmt.Printf("ioctl called.\n")
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

// InitialRegState GuestPhysAddr                      Binary files [+ offsets in the file]
//
//                 0x00000000    +------------------+
//                               |                  |
// RSI -->         0x00010000    +------------------+ bzImage [+ 0]
//                               |                  |
//                               |  boot protocol   |
//                               |                  |
//                               +------------------+
//                               |                  |
//                 0x00020000    +------------------+
//                               |                  |
//                               |   cmdline        |
//                               |                  |
//                               +------------------+
//                               |                  |
// RIP -->         0x00100000    +------------------+ bzImage [+ 512 x (setup_sects in boot protocol header)]
//                               |                  |
//                               |   64bit kernel   |
//                               |                  |
//                               +------------------+
//                               |                  |
//                 0x0f000000    +------------------+ initrd [+ 0]
//                               |                  |
//                               |   initrd         |
//                               |                  |
//                               +------------------+
//                               |                  |
//                 0x40000000    +------------------+
const (
	memSize = 1 << 30

	bootParamAddr = 0x10000
	cmdlineAddr   = 0x20000

	kernelAddr = 0x100000
	initrdAddr = 0xf000000
)

// loadflags.
const (
	LoadedHigh   = uint8(1 << 0)
	KASLRFlag    = uint8(1 << 1)
	QuietFlag    = uint8(1 << 5)
	KeepSegments = uint8(1 << 6)
	CanUseHeap   = uint8(1 << 7)
)

type LinuxGuest struct {
	kvmFd, vmFd, vcpuFd uintptr
	mem                 []byte
	run                 *RunData
	serial              *serial.Serial
}

func NewLinuxGuest(bzImagePath, initPath string) (*LinuxGuest, error) {
	g := &LinuxGuest{}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		panic(err)
	}

	g.kvmFd = devKVM.Fd()
	g.vmFd, err = CreateVM(g.kvmFd)

	if err != nil {
		panic(err)
	}

	if err := SetTSSAddr(g.vmFd); err != nil {
		panic(err)
	}

	if err := SetIdentityMapAddr(g.vmFd); err != nil {
		panic(err)
	}

	if err := CreateIRQChip(g.vmFd); err != nil {
		panic(err)
	}

	if err := CreatePIT2(g.vmFd); err != nil {
		panic(err)
	}

	g.vcpuFd, err = CreateVCPU(g.vmFd)
	if err != nil {
		panic(err)
	}

	if err := g.initCPUID(); err != nil {
		panic(err)
	}

	g.mem, err = syscall.Mmap(-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		panic(err)
	}

	mmapSize, err := GetVCPUMMmapSize(g.kvmFd)
	if err != nil {
		panic(err)
	}

	r, err := syscall.Mmap(int(g.vcpuFd), 0, int(mmapSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		panic(err)
	}

	g.run = (*RunData)(unsafe.Pointer(&r[0]))

	err = SetUserMemoryRegion(g.vmFd, &UserspaceMemoryRegion{
		Slot: 0, Flags: 0, GuestPhysAddr: 0, MemorySize: 1 << 30,
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&g.mem[0]))),
	})
	if err != nil {
		panic(err)
	}

	// Load initrd
	initrd, err := ioutil.ReadFile(initPath)
	if err != nil {
		return g, err
	}

	for i := 0; i < len(initrd); i++ {
		g.mem[initrdAddr+i] = initrd[i]
	}

	// Load cmdline
	cmdline := "console=ttyS0"
	for i, b := range []byte(cmdline) {
		g.mem[cmdlineAddr+i] = b
	}

	g.mem[cmdlineAddr+len(cmdline)] = 0 // for null terminated string

	// Load Boot Parameter
	bootProto, err := bootproto.New(bzImagePath)
	if err != nil {
		return g, err
	}

	bootProto.VidMode = 0xFFFF
	bootProto.TypeOfLoader = 0xFF
	bootProto.RamdiskImage = initrdAddr
	bootProto.RamdiskSize = uint32(len(initrd))
	bootProto.LoadFlags |= CanUseHeap | LoadedHigh | KeepSegments
	bootProto.HeapEndPtr = 0xFE00
	bootProto.ExtLoaderVer = 0
	bootProto.CmdlinePtr = cmdlineAddr
	bootProto.CmdlineSize = uint32(len(cmdline) + 1)

	bytes, err := bootProto.Bytes()
	if err != nil {
		return g, err
	}

	for i, b := range bytes {
		g.mem[bootParamAddr+i] = b
	}

	// Load kernel
	bzImage, err := ioutil.ReadFile(bzImagePath)
	if err != nil {
		return g, err
	}

	offset := int(bootProto.SetupSects+1) * 512 // copy to g.mem with offest setupsz

	for i := 0; i < len(bzImage)-offset; i++ {
		g.mem[kernelAddr+i] = bzImage[offset+i]
	}

	if err = g.initRegs(); err != nil {
		return g, err
	}

	if err = g.initSregs(); err != nil {
		return g, err
	}

	serialIRQCallback := func(irq, level uint32) {
		if err := IRQLine(g.vmFd, irq, level); err != nil {
			panic(err)
		}
	}

	if g.serial, err = serial.New(serialIRQCallback); err != nil {
		return g, err
	}

	return g, nil
}

func (g *LinuxGuest) GetInputChan() chan<- byte {
	return g.serial.GetInputChan()
}

func (g *LinuxGuest) InjectSerialIRQ() {
	g.serial.InjectIRQ()
}

func (g *LinuxGuest) initRegs() error {
	regs, err := GetRegs(g.vcpuFd)
	if err != nil {
		return err
	}

	regs.RFLAGS = 2
	regs.RIP = 0x100000
	regs.RSI = 0x10000

	if err := SetRegs(g.vcpuFd, regs); err != nil {
		return err
	}

	return nil
}

func (g *LinuxGuest) initSregs() error {
	sregs, err := GetSregs(g.vcpuFd)
	if err != nil {
		return err
	}

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

func (g *LinuxGuest) initCPUID() error {
	cpuid := CPUID{}
	cpuid.Nent = 100

	if err := GetSupportedCPUID(g.kvmFd, &cpuid); err != nil {
		return err
	}

	// https://www.kernel.org/doc/html/latest/virt/kvm/cpuid.html
	for i := 0; i < int(cpuid.Nent); i++ {
		if cpuid.Entries[i].Function != CPUIDSignature {
			continue
		}

		cpuid.Entries[i].Eax = CPUIDFeatures
		cpuid.Entries[i].Ebx = 0x4b4d564b // KVMK
		cpuid.Entries[i].Ecx = 0x564b4d56 // VMKV
		cpuid.Entries[i].Edx = 0x4d       // M
	}

	if err := SetCPUID2(g.vcpuFd, &cpuid); err != nil {
		return err
	}

	return nil
}

var ErrorUnexpectedEXITReason = errors.New("unexpected kvm exit reason")

func (g *LinuxGuest) RunInfiniteLoop() error {
	for {
		isContinute, err := g.RunOnce()
		if err != nil {
			return err
		}

		if !isContinute {
			return nil
		}
	}
}

func (g *LinuxGuest) RunOnce() (bool, error) {
	if err := Run(g.vcpuFd); err != nil {
		return false, err
	}

	switch g.run.ExitReason {
	case EXITHLT:
		fmt.Println("KVM_EXIT_HLT")

		return false, nil
	case EXITIO:
		direction, size, port, count, offset := g.run.IO()
		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(g.run)) + uintptr(offset))))[0:size]

		for i := 0; i < int(count); i++ {
			if err := g.handleExitIO(direction, port, bytes); err != nil {
				return false, err
			}
		}

		return true, nil
	default:
		return false, fmt.Errorf("%w: %d", ErrorUnexpectedEXITReason, g.run.ExitReason)
	}
}

func (g *LinuxGuest) handleExitIO(direction, port uint64, bytes []byte) error {
	switch {
	case 0x3c0 <= port && port <= 0x3da:
		return nil // VGA
	case 0x60 <= port && port <= 0x6F:
		return nil // PS/2 Keyboard (Always 8042 Chip)
	case 0x70 <= port && port <= 0x71:
		return nil // CMOS clock
	case 0x80 <= port && port <= 0x9F:
		return nil // DMA Page Registers (Commonly 74L612 Chip)
	case 0x2f8 <= port && port <= 0x2FF:
		return nil // Serial port 2
	case 0x3e8 <= port && port <= 0x3ef:
		return nil // Serial port 3
	case 0x2e8 <= port && port <= 0x2ef:
		return nil // Serial port 4
	case serial.COM1Addr <= port && port < serial.COM1Addr+8:
		if direction == EXITIOIN {
			return g.serial.In(port, bytes)
		}

		return g.serial.Out(port, bytes)
	default:
		return fmt.Errorf("%w: unexpected io port 0x%x", ErrorUnexpectedEXITReason, port)
	}
}
