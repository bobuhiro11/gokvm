package machine

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"
	"unsafe"

	"github.com/nmi/gokvm/bootproto"
	"github.com/nmi/gokvm/kvm"
	"github.com/nmi/gokvm/serial"
)

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
	kernelAddr    = 0x100000
	initrdAddr    = 0xf000000
)

type Machine struct {
	kvmFd, vmFd, vcpuFd uintptr
	mem                 []byte
	run                 *kvm.RunData
	serial              *serial.Serial
	ioportHandlers      [0x10000][2]func(m *Machine, port uint64, bytes []byte) error
}

func New() (*Machine, error) {
	m := &Machine{}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		return m, err
	}

	m.kvmFd = devKVM.Fd()
	m.vmFd, err = kvm.CreateVM(m.kvmFd)

	if err != nil {
		return m, err
	}

	if err := kvm.SetTSSAddr(m.vmFd); err != nil {
		return m, err
	}

	if err := kvm.SetIdentityMapAddr(m.vmFd); err != nil {
		return m, err
	}

	if err := kvm.CreateIRQChip(m.vmFd); err != nil {
		return m, err
	}

	if err := kvm.CreatePIT2(m.vmFd); err != nil {
		return m, err
	}

	m.vcpuFd, err = kvm.CreateVCPU(m.vmFd)
	if err != nil {
		return m, err
	}

	if err := m.initCPUID(); err != nil {
		return m, err
	}

	m.mem, err = syscall.Mmap(-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		return m, err
	}

	mmapSize, err := kvm.GetVCPUMMmapSize(m.kvmFd)
	if err != nil {
		return m, err
	}

	r, err := syscall.Mmap(int(m.vcpuFd), 0, int(mmapSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return m, err
	}

	m.run = (*kvm.RunData)(unsafe.Pointer(&r[0]))

	err = kvm.SetUserMemoryRegion(m.vmFd, &kvm.UserspaceMemoryRegion{
		Slot: 0, Flags: 0, GuestPhysAddr: 0, MemorySize: 1 << 30,
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&m.mem[0]))),
	})
	if err != nil {
		return m, err
	}

	return m, nil
}

func (m *Machine) LoadLinux(bzImagePath, initPath, params string) error {
	// Load initrd
	initrd, err := ioutil.ReadFile(initPath)
	if err != nil {
		return err
	}

	for i := 0; i < len(initrd); i++ {
		m.mem[initrdAddr+i] = initrd[i]
	}

	// Load kernel command-line parameters
	for i, b := range []byte(params) {
		m.mem[cmdlineAddr+i] = b
	}

	m.mem[cmdlineAddr+len(params)] = 0 // for null terminated string

	// Load Boot Parameter
	bootProto, err := bootproto.New(bzImagePath)
	if err != nil {
		return err
	}

	bootProto.VidMode = 0xFFFF
	bootProto.TypeOfLoader = 0xFF
	bootProto.RamdiskImage = initrdAddr
	bootProto.RamdiskSize = uint32(len(initrd))
	bootProto.LoadFlags |= bootproto.CanUseHeap | bootproto.LoadedHigh | bootproto.KeepSegments
	bootProto.HeapEndPtr = 0xFE00
	bootProto.ExtLoaderVer = 0
	bootProto.CmdlinePtr = cmdlineAddr
	bootProto.CmdlineSize = uint32(len(params) + 1)

	bytes, err := bootProto.Bytes()
	if err != nil {
		return err
	}

	for i, b := range bytes {
		m.mem[bootParamAddr+i] = b
	}

	// Load kernel
	bzImage, err := ioutil.ReadFile(bzImagePath)
	if err != nil {
		return err
	}

	offset := int(bootProto.SetupSects+1) * 512 // copy to g.mem with offest setupsz

	for i := 0; i < len(bzImage)-offset; i++ {
		m.mem[kernelAddr+i] = bzImage[offset+i]
	}

	if err = m.initRegs(); err != nil {
		return err
	}

	if err = m.initSregs(); err != nil {
		return err
	}

	m.initIOPortHandlers()

	serialIRQCallback := func(irq, level uint32) {
		if err := kvm.IRQLine(m.vmFd, irq, level); err != nil {
			panic(err)
		}
	}

	if m.serial, err = serial.New(serialIRQCallback); err != nil {
		return err
	}

	return nil
}

func (m *Machine) GetInputChan() chan<- byte {
	return m.serial.GetInputChan()
}

func (m *Machine) InjectSerialIRQ() {
	m.serial.InjectIRQ()
}

func (m *Machine) initRegs() error {
	regs, err := kvm.GetRegs(m.vcpuFd)
	if err != nil {
		return err
	}

	regs.RFLAGS = 2
	regs.RIP = 0x100000
	regs.RSI = 0x10000

	if err := kvm.SetRegs(m.vcpuFd, regs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initSregs() error {
	sregs, err := kvm.GetSregs(m.vcpuFd)
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

	if err := kvm.SetSregs(m.vcpuFd, sregs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initCPUID() error {
	cpuid := kvm.CPUID{}
	cpuid.Nent = 100

	if err := kvm.GetSupportedCPUID(m.kvmFd, &cpuid); err != nil {
		return err
	}

	// https://www.kernel.org/doc/html/latest/virt/kvm/cpuid.html
	for i := 0; i < int(cpuid.Nent); i++ {
		if cpuid.Entries[i].Function != kvm.CPUIDSignature {
			continue
		}

		cpuid.Entries[i].Eax = kvm.CPUIDFeatures
		cpuid.Entries[i].Ebx = 0x4b4d564b // KVMK
		cpuid.Entries[i].Ecx = 0x564b4d56 // VMKV
		cpuid.Entries[i].Edx = 0x4d       // M
	}

	if err := kvm.SetCPUID2(m.vcpuFd, &cpuid); err != nil {
		return err
	}

	return nil
}

func (m *Machine) RunInfiniteLoop() error {
	for {
		isContinute, err := m.RunOnce()
		if err != nil {
			return err
		}

		if !isContinute {
			return nil
		}
	}
}

func (m *Machine) RunOnce() (bool, error) {
	if err := kvm.Run(m.vcpuFd); err != nil {
		// When a signal is sent to the thread hosting the VM it will result in EINTR
		// refs https://gist.github.com/mcastelino/df7e65ade874f6890f618dc51778d83a
		if m.run.ExitReason == kvm.EXITINTR {
			return true, nil
		}

		return false, err
	}

	switch m.run.ExitReason {
	case kvm.EXITHLT:
		fmt.Println("KVM_EXIT_HLT")

		return false, nil
	case kvm.EXITIO:
		direction, size, port, count, offset := m.run.IO()
		f := m.ioportHandlers[port][direction]
		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(m.run)) + uintptr(offset))))[0:size]

		for i := 0; i < int(count); i++ {
			if err := f(m, port, bytes); err != nil {
				return false, err
			}
		}

		return true, nil
	default:
		return false, fmt.Errorf("%w: %d", kvm.ErrorUnexpectedEXITReason, m.run.ExitReason)
	}
}

func (m *Machine) initIOPortHandlers() {
	funcNone := func(m *Machine, port uint64, bytes []byte) error {
		return nil
	}

	funcError := func(m *Machine, port uint64, bytes []byte) error {
		return fmt.Errorf("%w: unexpected io port 0x%x", kvm.ErrorUnexpectedEXITReason, port)
	}

	// default handler
	for port := 0; port < 0x10000; port++ {
		for dir := kvm.EXITIOIN; dir <= kvm.EXITIOOUT; dir++ {
			m.ioportHandlers[port][dir] = funcError
		}
	}

	for dir := kvm.EXITIOIN; dir <= kvm.EXITIOOUT; dir++ {
		// VGA
		for port := 0x3c0; port <= 0x3da; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}

		for port := 0x3b4; port <= 0x3b5; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}

		// CMOS clock
		for port := 0x70; port <= 0x71; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}

		// DMA Page Registers (Commonly 74L612 Chip)
		for port := 0x80; port <= 0x9f; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}

		// Serial port 2
		for port := 0x2f8; port <= 0x2ff; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}

		// Serial port 3
		for port := 0x3e8; port <= 0x3ef; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}

		// Serial port 4
		for port := 0x2e8; port <= 0x2ef; port++ {
			m.ioportHandlers[port][dir] = funcNone
		}
	}

	// PS/2 Keyboard (Always 8042 Chip)
	for port := 0x60; port <= 0x6f; port++ {
		m.ioportHandlers[port][kvm.EXITIOIN] = func(m *Machine, port uint64, bytes []byte) error {
			// In ubuntu 20.04 on wsl2, the output to IO port 0x64 continued
			// infinitely. To deal with this issue, refer to kvmtool and
			// configure the input to the Status Register of the PS2 controller.
			//
			// refs:
			// https://github.com/kvmtool/kvmtool/blob/0e1882a49f81cb15d328ef83a78849c0ea26eecc/hw/i8042.c#L312
			// https://git.kernel.org/pub/scm/linux/kernel/git/will/kvmtool.git/tree/hw/i8042.c#n312
			// https://wiki.osdev.org/%228042%22_PS/2_Controller
			bytes[0] = 0x20

			return nil
		}
		m.ioportHandlers[port][kvm.EXITIOOUT] = funcNone
	}

	// Serial port 1
	for port := serial.COM1Addr; port < serial.COM1Addr+8; port++ {
		m.ioportHandlers[port][kvm.EXITIOIN] = func(m *Machine, port uint64, bytes []byte) error {
			return m.serial.In(port, bytes)
		}
		m.ioportHandlers[port][kvm.EXITIOOUT] = func(m *Machine, port uint64, bytes []byte) error {
			return m.serial.Out(port, bytes)
		}
	}
}
