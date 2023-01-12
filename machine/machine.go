package machine

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/bobuhiro11/gokvm/bootparam"
	"github.com/bobuhiro11/gokvm/ebda"
	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/pci"
	"github.com/bobuhiro11/gokvm/serial"
	"github.com/bobuhiro11/gokvm/tap"
	"github.com/bobuhiro11/gokvm/virtio"
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

	serialIRQ    = 4
	virtioNetIRQ = 9
	virtioBlkIRQ = 10
)

// ErrorWriteToCF9 indicates a write to cf9, the standard x86 reset port.
var ErrorWriteToCF9 = fmt.Errorf("power cycle via 0xcf9")

type Machine struct {
	kvmFd, vmFd    uintptr
	vcpuFds        []uintptr
	mem            []byte
	runs           []*kvm.RunData
	pci            *pci.PCI
	serial         *serial.Serial
	ioportHandlers [0x10000][2]func(port uint64, bytes []byte) error
}

func New(kvmPath string, nCpus int, tapIfName string, diskPath string) (*Machine, error) {
	m := &Machine{}

	devKVM, err := os.OpenFile(kvmPath, os.O_RDWR, 0o644)
	if err != nil {
		return m, err
	}

	m.kvmFd = devKVM.Fd()
	m.vcpuFds = make([]uintptr, nCpus)
	m.runs = make([]*kvm.RunData, nCpus)

	if m.vmFd, err = kvm.CreateVM(m.kvmFd); err != nil {
		return m, fmt.Errorf("CreateVM: %w", err)
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

	e, err := ebda.New(nCpus)
	if err != nil {
		return m, err
	}

	bytes, err := e.Bytes()
	if err != nil {
		return m, err
	}

	copy(m.mem[bootparam.EBDAStart:], bytes)

	m.pci = pci.New(pci.NewBridge()) // 00:00.0 for PCI bridge

	if len(tapIfName) > 0 {
		t, err := tap.New(tapIfName)
		if err != nil {
			return nil, err
		}

		v := virtio.NewNet(virtioNetIRQ, m, t, m.mem)
		go v.TxThreadEntry()
		go v.RxThreadEntry()
		// 00:01.0 for Virtio net
		m.pci.Devices = append(m.pci.Devices, v)
	}

	if len(diskPath) > 0 {
		v, err := virtio.NewBlk(diskPath, virtioBlkIRQ, m, m.mem)
		if err != nil {
			return nil, err
		}

		go v.IOThreadEntry()
		// 00:02.0 for Virtio blk
		m.pci.Devices = append(m.pci.Devices, v)
	}

	return m, nil
}

// RunData returns the kvm.RunData for the VM.
func (m *Machine) RunData() []*kvm.RunData {
	return m.runs
}

func (m *Machine) LoadLinux(kernel, initrd io.ReaderAt, params string) error {
	// Load initrd
	initrdSize, err := initrd.ReadAt(m.mem[initrdAddr:], 0)
	if err != nil && initrdSize == 0 && !errors.Is(err, io.EOF) {
		return fmt.Errorf("initrd: (%v, %w)", initrdSize, err)
	}

	// Load kernel command-line parameters
	copy(m.mem[cmdlineAddr:], params)
	m.mem[cmdlineAddr+len(params)] = 0 // for null terminated string

	// Load Boot Param
	bootParam, err := bootparam.New(kernel)
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
	bootParam.Hdr.RamdiskSize = uint32(initrdSize)                                                  // Proto 2.00+
	bootParam.Hdr.LoadFlags |= bootparam.CanUseHeap | bootparam.LoadedHigh | bootparam.KeepSegments // Proto 2.00+
	bootParam.Hdr.HeapEndPtr = 0xFE00                                                               // Proto 2.01+
	bootParam.Hdr.ExtLoaderVer = 0                                                                  // Proto 2.02+
	bootParam.Hdr.CmdlinePtr = cmdlineAddr                                                          // Proto 2.06+
	bootParam.Hdr.CmdlineSize = uint32(len(params) + 1)                                             // Proto 2.06+

	bytes, err := bootParam.Bytes()
	if err != nil {
		return err
	}

	copy(m.mem[bootParamAddr:], bytes)

	// Load kernel
	// copy to g.mem with offest setupsz
	//
	// The 32-bit (non-real-mode) kernel starts at offset (setup_sects+1)*512 in
	// the kernel file (again, if setup_sects == 0 the real value is 4.) It should
	// be loaded at address 0x10000 for Image/zImage kernels and 0x100000 for bzImage kernels.
	//
	// refs: https://www.kernel.org/doc/html/latest/x86/boot.html#loading-the-rest-of-the-kernel
	offset := int(bootParam.Hdr.SetupSects+1) * 512

	kernSize, err := kernel.ReadAt(m.mem[kernelAddr:], int64(offset))
	if err != nil && kernSize == 0 && !errors.Is(err, io.EOF) {
		return fmt.Errorf("kernel: (%v, %w)", kernSize, err)
	}

	for i := range m.vcpuFds {
		if err = m.initRegs(i); err != nil {
			return err
		}

		if err = m.initSregs(i); err != nil {
			return err
		}
	}

	if m.serial, err = serial.New(m); err != nil {
		return err
	}

	m.initIOPortHandlers()

	return nil
}

func (m *Machine) GetInputChan() chan<- byte {
	return m.serial.GetInputChan()
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

func (m *Machine) initCPUID(i int) error {
	cpuid := kvm.CPUID{}
	cpuid.Nent = 100

	if err := kvm.GetSupportedCPUID(m.kvmFd, &cpuid); err != nil {
		return err
	}

	// https://www.kernel.org/doc/html/latest/virt/kvm/cpuid.html
	for i := 0; i < int(cpuid.Nent); i++ {
		if cpuid.Entries[i].Function == kvm.CPUIDFuncPerMon {
			cpuid.Entries[i].Eax = 0 // disable
		} else if cpuid.Entries[i].Function == kvm.CPUIDSignature {
			cpuid.Entries[i].Eax = kvm.CPUIDFeatures
			cpuid.Entries[i].Ebx = 0x4b4d564b // KVMK
			cpuid.Entries[i].Ecx = 0x564b4d56 // VMKV
			cpuid.Entries[i].Edx = 0x4d       // M
		}
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
	err := kvm.Run(m.vcpuFds[i])

	exit := kvm.ExitType(m.runs[i].ExitReason)

	switch exit {
	case kvm.EXITHLT:
		fmt.Println("KVM_EXIT_HLT")

		return false, err
	case kvm.EXITIO:
		direction, size, port, count, offset := m.runs[i].IO()
		f := m.ioportHandlers[port][direction]
		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(m.runs[i])) + uintptr(offset))))[0:size]

		for i := 0; i < int(count); i++ {
			if err := f(port, bytes); err != nil {
				return false, err
			}
		}

		return true, err
	case kvm.EXITUNKNOWN:
		return true, err
	case kvm.EXITINTR:
		// When a signal is sent to the thread hosting the VM it will result in EINTR
		// refs https://gist.github.com/mcastelino/df7e65ade874f6890f618dc51778d83a
		return true, nil
	case kvm.EXITDCR,
		kvm.EXITDEBUG,
		kvm.EXITEXCEPTION,
		kvm.EXITFAILENTRY,
		kvm.EXITHYPERCALL,
		kvm.EXITINTERNALERROR,
		kvm.EXITIRQWINDOWOPEN,
		kvm.EXITMMIO,
		kvm.EXITNMI,
		kvm.EXITS390RESET,
		kvm.EXITS390SIEIC,
		kvm.EXITSETTPR,
		kvm.EXITSHUTDOWN,
		kvm.EXITTPRACCESS:
		if err != nil {
			return false, err
		}

		return false, fmt.Errorf("%w: %s", kvm.ErrUnexpectedEXITReason, exit.String())
	default:
		if err != nil {
			return false, err
		}

		return false, fmt.Errorf("%w: %s", kvm.ErrUnexpectedEXITReason, exit.String())
	}
}

func (m *Machine) registerIOPortHandler(
	start, end uint64,
	inHandler, outHandler func(port uint64, bytes []byte) error,
) {
	for i := start; i < end; i++ {
		m.ioportHandlers[i][kvm.EXITIOIN] = inHandler
		m.ioportHandlers[i][kvm.EXITIOOUT] = outHandler
	}
}

func (m *Machine) initIOPortHandlers() {
	funcNone := func(port uint64, bytes []byte) error {
		return nil
	}

	funcError := func(port uint64, bytes []byte) error {
		return fmt.Errorf("%w: unexpected io port 0x%x", kvm.ErrUnexpectedEXITReason, port)
	}

	// 0xCF9 port can get three values for three types of reset:
	//
	// Writing 4 to 0xCF9:(INIT) Will INIT the CPU. Meaning it will jump
	// to the initial location of booting but it will keep many CPU
	// elements untouched. Most internal tables, chaches etc will remain
	// unchanged by the Init call (but may change during it).
	//
	// Writing 6 to 0xCF9:(RESET) Will RESET the CPU with all
	// internal tables caches etc cleared to initial state.
	//
	// Writing 0xE to 0xCF9:(RESTART) Will power cycle the mother board
	// with everything that comes with it.
	// For now, we will exit without regard to the value. Should we wish
	// to have more sophisticated cf9 handling, we will need to modify
	// gokvm a bit more.
	funcOutbCF9 := func(port uint64, bytes []byte) error {
		if len(bytes) == 1 && bytes[0] == 0xe {
			return fmt.Errorf("write 0xe to cf9: %w", ErrorWriteToCF9)
		}

		return fmt.Errorf("write %#x to cf9: %w", bytes, ErrorWriteToCF9)
	}

	// In ubuntu 20.04 on wsl2, the output to IO port 0x64 continued
	// infinitely. To deal with this issue, refer to kvmtool and
	// configure the input to the Status Register of the PS2 controller.
	//
	// refs:
	// https://github.com/kvmtool/kvmtool/blob/0e1882a49f81cb15d328ef83a78849c0ea26eecc/hw/i8042.c#L312
	// https://git.kernel.org/pub/scm/linux/kernel/git/will/kvmtool.git/tree/hw/i8042.c#n312
	// https://wiki.osdev.org/%228042%22_PS/2_Controller
	funcInbPS2 := func(port uint64, bytes []byte) error {
		bytes[0] = 0x20

		return nil
	}

	m.registerIOPortHandler(0, 0x10000, funcError, funcError)    // default handler
	m.registerIOPortHandler(0xcf9, 0xcfa, funcNone, funcOutbCF9) // CF9
	m.registerIOPortHandler(0x3c0, 0x3db, funcNone, funcNone)    // VGA
	m.registerIOPortHandler(0x3b4, 0x3b6, funcNone, funcNone)    // VGA
	m.registerIOPortHandler(0x70, 0x72, funcNone, funcNone)      // CMOS clock
	m.registerIOPortHandler(0x80, 0xa0, funcNone, funcNone)      // DMA Page Registers (Commonly 74L612 Chip)
	m.registerIOPortHandler(0x2f8, 0x300, funcNone, funcNone)    // Serial port 2
	m.registerIOPortHandler(0x3e8, 0x3f0, funcNone, funcNone)    // Serial port 3
	m.registerIOPortHandler(0x2e8, 0x2f0, funcNone, funcNone)    // Serial port 4
	m.registerIOPortHandler(0xcfe, 0xcff, funcNone, funcNone)    // unknown
	m.registerIOPortHandler(0xcfa, 0xcfc, funcNone, funcNone)    // unknown
	m.registerIOPortHandler(0xc000, 0xd000, funcNone, funcNone)  // PCI Configuration Space Access Mechanism #2
	m.registerIOPortHandler(0x60, 0x70, funcInbPS2, funcNone)    // PS/2 Keyboard (Always 8042 Chip)
	m.registerIOPortHandler(0xed, 0xee, funcNone, funcNone)      // 0xed is the new standard delay port.

	// Serial port 1
	m.registerIOPortHandler(serial.COM1Addr, serial.COM1Addr+8, m.serial.In, m.serial.Out)

	// PCI configuration
	//
	// 0xcf8 for address register for PCI Config Space
	// 0xcfc + 0xcff for data for PCI Config Space
	// see https://github.com/torvalds/linux/blob/master/arch/x86/pci/direct.c for more detail.
	m.registerIOPortHandler(0xcf8, 0xcf9, m.pci.PciConfAddrIn, m.pci.PciConfAddrOut)
	m.registerIOPortHandler(0xcfc, 0xd00, m.pci.PciConfDataIn, m.pci.PciConfDataOut)

	// PCI devices
	for i, device := range m.pci.Devices {
		start, end := device.GetIORange()
		m.registerIOPortHandler(
			start, end,
			m.pci.Devices[i].IOInHandler, m.pci.Devices[i].IOOutHandler,
		)
	}
}

func (m *Machine) InjectSerialIRQ() error {
	if err := kvm.IRQLine(m.vmFd, serialIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLine(m.vmFd, serialIRQ, 1); err != nil {
		return err
	}

	return nil
}

func (m *Machine) InjectVirtioNetIRQ() error {
	if err := kvm.IRQLine(m.vmFd, virtioNetIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLine(m.vmFd, virtioNetIRQ, 1); err != nil {
		return err
	}

	return nil
}

func (m *Machine) InjectVirtioBlkIRQ() error {
	if err := kvm.IRQLine(m.vmFd, virtioBlkIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLine(m.vmFd, virtioBlkIRQ, 1); err != nil {
		return err
	}

	return nil
}
