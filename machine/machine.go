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
	kvmFd, vmFd uintptr
	vcpuFds     []uintptr
	mem         []byte
	runs        []*kvm.RunData
	serial      *serial.Serial
}

func New(nCpus int) (*Machine, error) {
	m := &Machine{}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		return m, err
	}

	m.kvmFd = devKVM.Fd()
	m.vmFd, err = kvm.CreateVM(m.kvmFd)
	m.vcpuFds = make([]uintptr, nCpus)
	m.runs = make([]*kvm.RunData, nCpus)

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

	for i := 0; i < nCpus; i++ {
		m.vcpuFds[i], err = kvm.CreateVCPU(m.vmFd, i)
		if err != nil {
			return m, err
		}
	}

	for i := 0; i < nCpus; i++ {
		if err := m.initCPUID(i); err != nil {
			return m, err
		}
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

	for i := 0; i < nCpus; i++ {
		r, err := syscall.Mmap(int(m.vcpuFds[i]), 0, int(mmapSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return m, err
		}

		m.runs[i] = (*kvm.RunData)(unsafe.Pointer(&r[0]))
	}

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

	for i := range m.vcpuFds {
		if err = m.initRegs(i); err != nil {
			return err
		}

		if err = m.initSregs(i); err != nil {
			return err
		}
	}

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

func (m *Machine) initRegs(i int) error {
	regs, err := kvm.GetRegs(m.vcpuFds[i])
	if err != nil {
		return err
	}

	regs.RFLAGS = 2
	regs.RIP = 0x100000
	regs.RSI = 0x10000

	if err := kvm.SetRegs(m.vcpuFds[i], regs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initSregs(i int) error {
	sregs, err := kvm.GetSregs(m.vcpuFds[i])
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

	if err := kvm.SetSregs(m.vcpuFds[i], sregs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initCPUID(i int) error {
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

	if err := kvm.SetCPUID2(m.vcpuFds[i], &cpuid); err != nil {
		return err
	}

	return nil
}

func (m *Machine) RunInfiniteLoop(i int) error {
	for {
		isContinute, err := m.RunOnce(i)
		if err != nil {
			return err
		}

		if !isContinute {
			return nil
		}
	}
}

func (m *Machine) RunOnce(i int) (bool, error) {
	if err := kvm.Run(m.vcpuFds[i]); err != nil {
		// When a signal is sent to the thread hosting the VM it will result in EINTR
		// refs https://gist.github.com/mcastelino/df7e65ade874f6890f618dc51778d83a
		if m.runs[i].ExitReason == kvm.EXITINTR {
			return true, nil
		}

		return false, err
	}

	switch m.runs[i].ExitReason {
	case kvm.EXITHLT:
		fmt.Println("KVM_EXIT_HLT")

		return false, nil
	case kvm.EXITIO:
		direction, size, port, count, offset := m.runs[i].IO()
		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(m.runs[i])) + uintptr(offset))))[0:size]

		for i := 0; i < int(count); i++ {
			if err := m.handleExitIO(direction, port, bytes); err != nil {
				return false, err
			}
		}

		return true, nil
	default:
		return false, fmt.Errorf("%w: %d", kvm.ErrorUnexpectedEXITReason, m.runs[i].ExitReason)
	}
}

func (m *Machine) handleExitIO(direction, port uint64, bytes []byte) error {
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
		if direction == kvm.EXITIOIN {
			return m.serial.In(port, bytes)
		}

		return m.serial.Out(port, bytes)
	default:
		return fmt.Errorf("%w: unexpected io port 0x%x", kvm.ErrorUnexpectedEXITReason, port)
	}
}
