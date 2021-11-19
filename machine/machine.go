package machine

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/bootparam"
	"github.com/bobuhiro11/gokvm/ebda"
	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/serial"
)

// InitialRegState GuestPhysAddr                      Binary files [+ offsets in the file]
//
//                 0x00000000    +------------------+
//                               |                  |
// RSI -->         0x00010000    +------------------+ bzImage [+ 0]
//                               |                  |
//                               |  boot param      |
//                               |                  |
//                               +------------------+
//                               |                  |
//                 0x00020000    +------------------+
//                               |                  |
//                               |   cmdline        |
//                               |                  |
//                               +------------------+
//                               |                  |
// RIP -->         0x00100000    +------------------+ bzImage [+ 512 x (setup_sects in boot param header + 1)]
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
	memSize       = 1 << 30
	bootParamAddr = 0x10000
	cmdlineAddr   = 0x20000
	kernelAddr    = 0x100000
	initrdAddr    = 0xf000000
)

type Machine struct {
	kvmFd, vmFd    uintptr
	vcpuFds        []uintptr
	mem            []byte
	runs           []*kvm.RunData
	serial         *serial.Serial
	ioportHandlers [0x10000][2]func(m *Machine, port uint64, bytes []byte) error
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

	mmapSize, err := kvm.GetVCPUMMmapSize(m.kvmFd)
	if err != nil {
		return m, err
	}

	for i := 0; i < nCpus; i++ {
		// Create vCPU
		m.vcpuFds[i], err = kvm.CreateVCPU(m.vmFd, i)
		if err != nil {
			return m, err
		}

		// init CPUID
		if err := m.initCPUID(i); err != nil {
			return m, err
		}

		if err := m.initMSR(i); err != nil {
			return m, err
		}

		// init kvm_run structure
		r, err := syscall.Mmap(int(m.vcpuFds[i]), 0, int(mmapSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return m, err
		}

		m.runs[i] = (*kvm.RunData)(unsafe.Pointer(&r[0]))
	}

	m.mem, err = syscall.Mmap(-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		return m, err
	}

	err = kvm.SetUserMemoryRegion(m.vmFd, &kvm.UserspaceMemoryRegion{
		Slot: 0, Flags: 0, GuestPhysAddr: 0, MemorySize: 1 << 30,
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&m.mem[0]))),
	})
	if err != nil {
		return m, err
	}

	e, err := ebda.New()
	if err != nil {
		return m, err
	}

	bytes, err := e.Bytes()
	if err != nil {
		return m, err
	}

	for i, b := range bytes {
		m.mem[bootparam.EBDAStart+i] = b
	}

	return m, nil
}

// RunData returns the kvm.RunData for the VM.
func (m *Machine) RunData() []*kvm.RunData {
	return m.runs
}

const (
	APICModeExtInt = 0x7
	APICModeNMI    = 0x4
)

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

	// Load Boot Param
	bootParam, err := bootparam.New(bzImagePath)
	if err != nil {
		return err
	}

	// refs https://github.com/kvmtool/kvmtool/blob/0e1882a49f81cb15d328ef83a78849c0ea26eecc/x86/bios.c#L66-L86
	bootParam.AddE820Entry(
		bootparam.RealModeIvtBegin,
		bootparam.EBDAStart-bootparam.RealModeIvtBegin,
		bootparam.E820Ram,
	)
	bootParam.AddE820Entry(
		bootparam.EBDAStart,
		bootparam.VGARAMBegin-bootparam.EBDAStart,
		bootparam.E820Reserved,
	)
	bootParam.AddE820Entry(
		bootparam.MBBIOSBegin,
		bootparam.MBBIOSEnd-bootparam.MBBIOSBegin,
		bootparam.E820Reserved,
	)
	bootParam.AddE820Entry(
		kernelAddr,
		memSize-kernelAddr,
		bootparam.E820Ram,
	)

	bootParam.Hdr.VidMode = 0xFFFF                                                                  // Proto ALL
	bootParam.Hdr.TypeOfLoader = 0xFF                                                               // Proto 2.00+
	bootParam.Hdr.RamdiskImage = initrdAddr                                                         // Proto 2.00+
	bootParam.Hdr.RamdiskSize = uint32(len(initrd))                                                 // Proto 2.00+
	bootParam.Hdr.LoadFlags |= bootparam.CanUseHeap | bootparam.LoadedHigh | bootparam.KeepSegments // Proto 2.00+
	bootParam.Hdr.HeapEndPtr = 0xFE00                                                               // Proto 2.01+
	bootParam.Hdr.ExtLoaderVer = 0                                                                  // Proto 2.02+
	bootParam.Hdr.CmdlinePtr = cmdlineAddr                                                          // Proto 2.06+
	bootParam.Hdr.CmdlineSize = uint32(len(params) + 1)                                             // Proto 2.06+

	bytes, err := bootParam.Bytes()
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

	// copy to g.mem with offest setupsz
	//
	// The 32-bit (non-real-mode) kernel starts at offset (setup_sects+1)*512 in
	// the kernel file (again, if setup_sects == 0 the real value is 4.) It should
	// be loaded at address 0x10000 for Image/zImage kernels and 0x100000 for bzImage kernels.
	//
	// refs: https://www.kernel.org/doc/html/latest/x86/boot.html#loading-the-rest-of-the-kernel
	offset := int(bootParam.Hdr.SetupSects+1) * 512

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

		vcpuFd := m.vcpuFds[i]

		lapic, err := kvm.GetLAPIC(vcpuFd)
		if err != nil {
			return err
		}

		lapic.LvtLint0 |= APICModeExtInt << 8
		lapic.LvtLint1 |= APICModeNMI << 8

		err = kvm.SetLAPIC(vcpuFd, lapic)
		if err != nil {
			return err
		}
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

func (m *Machine) initRegs(i int) error {
	regs, err := kvm.GetRegs(m.vcpuFds[i])
	if err != nil {
		return err
	}

	regs.RFLAGS = 2
	regs.RIP = kernelAddr
	regs.RSI = bootParamAddr

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

const (
	MsrIA32SystemCS             = 0x00000174
	MsrIA32SystemESP            = 0x00000175
	MsrIA32SystemEIP            = 0x00000176
	MsrSTAR                     = 0xc0000081
	MsrCSTAR                    = 0xc0000083
	MsrKernelGSBase             = 0xc0000102
	MsrSyscallMask              = 0xc0000084
	MsrLSTAR                    = 0xc0000082
	MsrIA32TSC                  = 0x00000010
	MsrIA32MiscEnable           = 0x000001a0
	MsrIA32MiscEnableFastString = 1
)

func (m *Machine) initMSR(i int) error {
	msrs := kvm.MSRs{}

	msrs.Entries[0].Index = MsrIA32SystemCS
	msrs.Entries[0].Data = 0

	msrs.Entries[1].Index = MsrIA32SystemESP
	msrs.Entries[1].Data = 0

	msrs.Entries[2].Index = MsrIA32SystemEIP
	msrs.Entries[2].Data = 0

	msrs.Entries[3].Index = MsrSTAR
	msrs.Entries[3].Data = 0

	msrs.Entries[4].Index = MsrCSTAR
	msrs.Entries[4].Data = 0

	msrs.Entries[5].Index = MsrKernelGSBase
	msrs.Entries[5].Data = 0

	msrs.Entries[6].Index = MsrSyscallMask
	msrs.Entries[6].Data = 0

	msrs.Entries[7].Index = MsrLSTAR
	msrs.Entries[7].Data = 0

	msrs.Entries[8].Index = MsrIA32TSC
	msrs.Entries[8].Data = 0

	msrs.Entries[9].Index = MsrIA32MiscEnable
	msrs.Entries[9].Data = MsrIA32MiscEnableFastString

	msrs.NumOfMSRs = 10

	if err := kvm.SetMSRs(m.vcpuFds[i], msrs); err != nil {
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
	// https://www.kernel.org/doc/Documentation/virtual/kvm/api.txt
	// - vcpu ioctls: These query and set attributes that control the operation
	//   of a single virtual cpu.
	//
	//   vcpu ioctls should be issued from the same thread that was used to create
	//   the vcpu, except for asynchronous vcpu ioctl that are marked as such in
	//   the documentation.  Otherwise, the first ioctl after switching threads
	//   could see a performance impact.
	//
	// - device ioctls: These query and set attributes that control the operation
	//   of a single device.
	//
	//   device ioctls must be issued from the same process (address space) that
	//   was used to create the VM.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for {
		isContinue, err := m.RunOnce(i)
		if err != nil {
			return err
		}

		if !isContinue {
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
		f := m.ioportHandlers[port][direction]
		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(m.runs[i])) + uintptr(offset))))[0:size]

		for i := 0; i < int(count); i++ {
			if err := f(m, port, bytes); err != nil {
				return false, err
			}
		}

		return true, nil
	default:
		return false, fmt.Errorf("%w: %d", kvm.ErrorUnexpectedEXITReason, m.runs[i].ExitReason)
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
