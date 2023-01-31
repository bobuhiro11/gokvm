package machine

import (
	"bytes"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
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
	"golang.org/x/arch/x86/x86asm"
)

var ErrZeroSizeKernel = errors.New("kernel is 0 bytes")

// ErrWriteToCF9 indicates a write to cf9, the standard x86 reset port.
var ErrWriteToCF9 = fmt.Errorf("power cycle via 0xcf9")

// ErrBadVA indicates a bad virtual address was used.
var ErrBadVA = fmt.Errorf("bad virtual address")

// ErrBadCPU indicates a cpu number is invalid.
var ErrBadCPU = fmt.Errorf("bad cpu number")

// ErrUnsupported indicates something we do not yet do.
var ErrUnsupported = fmt.Errorf("unsupported")

// ErrMemTooSmall indicates the requested memory size is too small.
var ErrMemTooSmall = fmt.Errorf("mem request must be at least 1<<20")

type Machine struct {
	kvmFd, vmFd    uintptr
	vcpuFds        []uintptr
	mem            []byte
	runs           []*kvm.RunData
	pci            *pci.PCI
	serial         *serial.Serial
	ioportHandlers [0x10000][2]func(port uint64, bytes []byte) error
}

// New creates a new KVM. This includes opening the kvm device, creating VM, creating
// vCPUs, and attaching memory, disk (if needed), and tap (if needed).
func New(kvmPath string, nCpus int, tapIfName string, diskPath string, memSize int) (*Machine, error) {
	if memSize < MinMemSize {
		return nil, fmt.Errorf("memory size %d:%w", memSize, ErrMemTooSmall)
	}

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

	for cpu := 0; cpu < nCpus; cpu++ {
		// Create vCPU
		m.vcpuFds[cpu], err = kvm.CreateVCPU(m.vmFd, cpu)
		if err != nil {
			return m, err
		}

		// init CPUID
		if err := m.initCPUID(cpu); err != nil {
			return m, err
		}

		// init kvm_run structure
		r, err := syscall.Mmap(int(m.vcpuFds[cpu]), 0, int(mmapSize),
			syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if err != nil {
			return m, err
		}

		m.runs[cpu] = (*kvm.RunData)(unsafe.Pointer(&r[0]))
	}

	// Another coding anti-pattern reguired by golangci-lint.
	// Would not pass review in Google.
	if m.mem, err = syscall.Mmap(-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_ANONYMOUS); err != nil {
		return m, err
	}

	err = kvm.SetUserMemoryRegion(m.vmFd, &kvm.UserspaceMemoryRegion{
		Slot: 0, Flags: 0, GuestPhysAddr: 0, MemorySize: uint64(memSize),
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
	// Poison memory.
	// 0 is valid instruction and if you start running in the middle of all those
	// 0's it is impossible to diagnore.
	for i := highMemBase; i < len(m.mem); i += len(Poison) {
		copy(m.mem[i:], Poison)
	}

	return m, nil
}

// Translate translates a virtual address for all active CPUs
// and returns a []*Translate or error.
func (m *Machine) Translate(vaddr uint64) ([]*Translate, error) {
	t := make([]*Translate, 0, len(m.vcpuFds))

	for cpu := range m.vcpuFds {
		tt, err := GetTranslate(m.vcpuFds[cpu], vaddr)
		if err != nil {
			return t, err
		}

		t = append(t, tt)
	}

	return t, nil
}

// SetupRegs sets up the general purpose registers,
// including a RIP and BP.
func (m *Machine) SetupRegs(rip, bp uint64, amd64 bool) error {
	for _, cpu := range m.vcpuFds {
		if err := m.initRegs(cpu, rip, bp); err != nil {
			return err
		}

		if err := m.initSregs(cpu, amd64); err != nil {
			return err
		}
	}

	return nil
}

// RunData returns the kvm.RunData for the VM.
func (m *Machine) RunData() []*kvm.RunData {
	return m.runs
}

// LoadLinux loads a bzImage or ELF file, an optional initrd, and
// optional params.
func (m *Machine) LoadLinux(kernel, initrd io.ReaderAt, params string) error {
	var (
		DefaultKernelAddr = uint64(highMemBase)
		err               error
	)

	// Load initrd
	initrdSize, err := initrd.ReadAt(m.mem[initrdAddr:], 0)
	if err != nil && initrdSize == 0 && !errors.Is(err, io.EOF) {
		return fmt.Errorf("initrd: (%v, %w)", initrdSize, err)
	}

	// Load kernel command-line parameters
	copy(m.mem[cmdlineAddr:], params)
	m.mem[cmdlineAddr+len(params)] = 0 // for null terminated string

	// try to read as ELF. If it fails, no problem,
	// next effort is to read as a bzimage.
	var isElfFile bool

	k, err := elf.NewFile(kernel)
	if err == nil {
		isElfFile = true
	}

	bootParam := &bootparam.BootParam{}

	// might be a bzimage
	if !isElfFile {
		// Load Boot Param
		bootParam, err = bootparam.New(kernel)
		if err != nil {
			return err
		}
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
		highMemBase,
		uint64(len(m.mem)-highMemBase),
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

	var (
		amd64    bool
		kernSize int
	)

	switch isElfFile {
	case false:
		// Load kernel
		// copy to g.mem with offset setupsz
		//
		// The 32-bit (non-real-mode) kernel starts at offset (setup_sects+1)*512 in
		// the kernel file (again, if setup_sects == 0 the real value is 4.) It should
		// be loaded at address 0x10000 for Image/zImage kernels and highMemBase for bzImage kernels.
		//
		// refs: https://www.kernel.org/doc/html/latest/x86/boot.html#loading-the-rest-of-the-kernel
		setupsz := int(bootParam.Hdr.SetupSects+1) * 512

		kernSize, err = kernel.ReadAt(m.mem[DefaultKernelAddr:], int64(setupsz))

		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("kernel: (%v, %w)", kernSize, err)
		}
	case true:
		if k.Class == elf.ELFCLASS64 {
			amd64 = true
		}

		DefaultKernelAddr = k.Entry

		for i, p := range k.Progs {
			if p.Type != elf.PT_LOAD {
				continue
			}

			log.Printf("Load elf segment @%#x from file %#x %#x bytes", p.Paddr, p.Off, p.Filesz)

			n, err := p.ReadAt(m.mem[p.Paddr:], 0)
			if !errors.Is(err, io.EOF) || uint64(n) != p.Filesz {
				return fmt.Errorf("reading ELF prog %d@%#x: %d/%d bytes, err %w", i, p.Paddr, n, p.Filesz, err)
			}

			kernSize += n
		}
	}

	if kernSize == 0 {
		return ErrZeroSizeKernel
	}

	if err := m.SetupRegs(DefaultKernelAddr, bootParamAddr, amd64); err != nil {
		return err
	}

	if m.serial, err = serial.New(m); err != nil {
		return err
	}

	m.initIOPortHandlers()

	return nil
}

// GetInputChan returns a chan <- byte for serial.
func (m *Machine) GetInputChan() chan<- byte {
	return m.serial.GetInputChan()
}

// GetRegs gets regs for vCPU.
func (m *Machine) GetRegs(cpu int) (*kvm.Regs, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return nil, err
	}

	return kvm.GetRegs(fd)
}

// GetSRegs gets sregs for vCPU.
func (m *Machine) GetSRegs(cpu int) (*kvm.Sregs, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return nil, err
	}

	return kvm.GetSregs(fd)
}

// SetRegs sets regs for vCPU.
func (m *Machine) SetRegs(cpu int, r *kvm.Regs) error {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return err
	}

	return kvm.SetRegs(fd, r)
}

// SetSRegs sets sregs for vCPU.
func (m *Machine) SetSRegs(cpu int, s *kvm.Sregs) error {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return err
	}

	return kvm.SetSregs(fd, s)
}

func (m *Machine) initRegs(vcpufd uintptr, rip, bp uint64) error {
	regs, err := kvm.GetRegs(vcpufd)
	if err != nil {
		return err
	}

	// Clear all FLAGS bits, except bit 1 which is always set.
	regs.RFLAGS = 2
	regs.RIP = rip
	// Create stack which will grow down.
	regs.RSI = bp

	if err := kvm.SetRegs(vcpufd, regs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initSregs(vcpufd uintptr, amd64 bool) error {
	sregs, err := kvm.GetSregs(vcpufd)
	if err != nil {
		return err
	}

	if !amd64 {
		// set all segment flat
		sregs.CS.Base, sregs.CS.Limit, sregs.CS.G = 0, 0xFFFFFFFF, 1
		sregs.DS.Base, sregs.DS.Limit, sregs.DS.G = 0, 0xFFFFFFFF, 1
		sregs.FS.Base, sregs.FS.Limit, sregs.FS.G = 0, 0xFFFFFFFF, 1
		sregs.GS.Base, sregs.GS.Limit, sregs.GS.G = 0, 0xFFFFFFFF, 1
		sregs.ES.Base, sregs.ES.Limit, sregs.ES.G = 0, 0xFFFFFFFF, 1
		sregs.SS.Base, sregs.SS.Limit, sregs.SS.G = 0, 0xFFFFFFFF, 1

		sregs.CS.DB, sregs.SS.DB = 1, 1
		sregs.CR0 |= 1 // protected mode

		if err := kvm.SetSregs(vcpufd, sregs); err != nil {
			return err
		}

		return nil
	}

	high64k := m.mem[pageTableBase : pageTableBase+0x6000]

	// zero out the page tables.
	// but we might in fact want to poison them?
	// do we really want 1G, for example?
	for i := range high64k {
		high64k[i] = 0
	}

	// Set up page tables for long mode.
	// take the first six pages of an area it should not touch -- PageTableBase
	// present, read/write, page table at 0xffff0000
	// ptes[0] = PageTableBase + 0x1000 | 0x3
	// 3 in lowest 2 bits means present and read/write
	// 0x60 means accessed/dirty
	// 0x80 means the page size bit -- 0x80 | 0x60 = 0xe0
	// 0x10 here is making it point at the next page.
	// another go anti-pattern from golangci-lint.
	// golangci-lint claims this file has not been go-fumpt-ed
	// but it has.
	copy(high64k, []byte{
		0x03,
		0x10 | uint8((pageTableBase>>8)&0xff),
		uint8((pageTableBase >> 16) & 0xff),
		uint8((pageTableBase >> 24) & 0xff), 0, 0, 0, 0,
	})
	// need four pointers to 2M page tables -- PHYSICAL addresses:
	// 0x2000, 0x3000, 0x4000, 0x5000
	// experiment: set PS bit
	// Don't.
	for i := uint64(0); i < 4; i++ {
		ptb := pageTableBase + (i+2)*0x1000
		// Another coding anti-pattern
		copy(high64k[int(i*8)+0x1000:],
			[]byte{
				/*0x80 |*/ 0x63,
				uint8((ptb >> 8) & 0xff),
				uint8((ptb >> 16) & 0xff),
				uint8((ptb >> 24) & 0xff), 0, 0, 0, 0,
			})
	}
	// Now the 2M pages.
	for i := uint64(0); i < 0x1_0000_0000; i += 0x2_00_000 {
		ptb := i | 0xe3
		ix := int((i/0x2_00_000)*8 + 0x2000)
		// another coding anti-pattern from golangci-lint.
		copy(high64k[ix:], []byte{
			uint8(ptb),
			uint8((ptb >> 8) & 0xff),
			uint8((ptb >> 16) & 0xff),
			uint8((ptb >> 24) & 0xff), 0, 0, 0, 0,
		})
	}

	// set to true to debug.
	if false {
		log.Printf("Page tables: %s", hex.Dump(m.mem[pageTableBase:pageTableBase+0x3000]))
	}

	sregs.CR3 = uint64(pageTableBase)
	sregs.CR4 = CR4xPAE
	sregs.CR0 = CR0xPE | CR0xMP | CR0xET | CR0xNE | CR0xWP | CR0xAM | CR0xPG
	sregs.EFER = EFERxLME | EFERxLMA

	seg := kvm.Segment{
		Base:     0,
		Limit:    0xffffffff,
		Selector: 1 << 3,
		Typ:      11, /* Code: execute, read, accessed */
		Present:  1,
		DPL:      0,
		DB:       0,
		S:        1, /* Code/data */
		L:        1,
		G:        1, /* 4KB granularity */
		AVL:      0,
	}

	sregs.CS = seg

	seg.Typ = 3 /* Data: read/write, accessed */
	seg.Selector = 2 << 3
	sregs.DS, sregs.ES, sregs.FS, sregs.GS, sregs.SS = seg, seg, seg, seg, seg

	if err := kvm.SetSregs(vcpufd, sregs); err != nil {
		return err
	}

	return nil
}

func (m *Machine) initCPUID(cpu int) error {
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

	if err := kvm.SetCPUID2(m.vcpuFds[cpu], &cpuid); err != nil {
		return err
	}

	return nil
}

// SingleStep enables single stepping the guest.
func (m *Machine) SingleStep(onoff bool) error {
	for cpu := range m.vcpuFds {
		if err := kvm.SingleStep(m.vcpuFds[cpu], onoff); err != nil {
			return fmt.Errorf("single step %d:%w", cpu, err)
		}
	}

	return nil
}

// RunInfiniteLoop runs the guest cpu until there is an error.
// If the error is ErrExitDebug, this function can be called again.
func (m *Machine) RunInfiniteLoop(cpu int) error {
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
		isContinue, err := m.RunOnce(cpu)
		if isContinue {
			if err != nil {
				fmt.Printf("%v\r\n", err)
			}

			continue
		}

		if err != nil {
			return err
		}
	}
}

// RunOnce runs the guest vCPU until it exits.
func (m *Machine) RunOnce(cpu int) (bool, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return false, err
	}

	_ = kvm.Run(fd)
	exit := kvm.ExitType(m.runs[cpu].ExitReason)

	switch exit {
	case kvm.EXITHLT:
		return false, err

	case kvm.EXITIO:
		direction, size, port, count, offset := m.runs[cpu].IO()
		f := m.ioportHandlers[port][direction]
		bytes := (*(*[100]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(m.runs[cpu])) + uintptr(offset))))[0:size]

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
	case kvm.EXITDEBUG:
		return false, kvm.ErrDebug

	case kvm.EXITDCR,
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

		return false, fmt.Errorf("%w: %s", kvm.ErrUnexpectedExitReason, exit.String())
	default:
		if err != nil {
			return false, err
		}

		r, _ := m.GetRegs(cpu)
		s, _ := m.GetSRegs(cpu)
		// another coding anti-pattern from golangci-lint.
		return false, fmt.Errorf("%w: %v: regs:\n%s",
			kvm.ErrUnexpectedExitReason,
			kvm.ExitType(m.runs[cpu].ExitReason).String(), show("", &s, &r))
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
		return fmt.Errorf("%w: unexpected io port 0x%x", kvm.ErrUnexpectedExitReason, port)
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
			return fmt.Errorf("write 0xe to cf9: %w", ErrWriteToCF9)
		}

		return fmt.Errorf("write %#x to cf9: %w", bytes, ErrWriteToCF9)
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

// InjectSerialIRQ injects a serial interrupt.
func (m *Machine) InjectSerialIRQ() error {
	if err := kvm.IRQLine(m.vmFd, serialIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLine(m.vmFd, serialIRQ, 1); err != nil {
		return err
	}

	return nil
}

// InjectViortNetIRQ injects a virtio net interrupt.
func (m *Machine) InjectVirtioNetIRQ() error {
	if err := kvm.IRQLine(m.vmFd, virtioNetIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLine(m.vmFd, virtioNetIRQ, 1); err != nil {
		return err
	}

	return nil
}

// InjectViortNetIRQ injects a virtio block interrupt.
func (m *Machine) InjectVirtioBlkIRQ() error {
	if err := kvm.IRQLine(m.vmFd, virtioBlkIRQ, 0); err != nil {
		return err
	}

	if err := kvm.IRQLine(m.vmFd, virtioBlkIRQ, 1); err != nil {
		return err
	}

	return nil
}

// ReadAt implements io.ReadAt for the kvm guest memory.
func (m *Machine) ReadAt(b []byte, off int64) (int, error) {
	mem := bytes.NewReader(m.mem)

	return mem.ReadAt(b, off)
}

// WriteAt implements io.WriteAt for the kvm guest memory.
func (m *Machine) WriteAt(b []byte, off int64) (int, error) {
	if off > int64(len(m.mem)) {
		return 0, syscall.EFBIG
	}

	n := copy(m.mem[off:], b)

	return n, nil
}

func showone(indent string, in interface{}) string {
	var ret string

	s := reflect.ValueOf(in).Elem()
	typeOfT := s.Type()

	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		if f.Kind() == reflect.String {
			ret += fmt.Sprintf(indent+"%s %s = %s\n", typeOfT.Field(i).Name, f.Type(), f.Interface())
		} else {
			ret += fmt.Sprintf(indent+"%s %s = %#x\n", typeOfT.Field(i).Name, f.Type(), f.Interface())
		}
	}

	return ret
}

func show(indent string, l ...interface{}) string {
	var ret string
	for _, i := range l {
		ret += showone(indent, i)
	}

	return ret
}

// Translate is a struct for KVM_TRANSLATE queries.
type Translate struct {
	// LinearAddress is input.
	// Most people call this a "virtual address"
	// Intel has their own name.
	LinearAddress uint64

	// This is output
	PhysicalAddress uint64
	Valid           uint8
	Writeable       uint8
	Usermode        uint8
	_               [5]uint8
}

// GetTranslate returns the virtual to physical mapping across all vCPUs.
// It is incredibly helpful for debugging at startup and detecting
// corrupted page tables.
// N.B.: on x86 it appears to ignore vcpufd.
// And, further, it always says the address is valid.
// I've no idea why.
func GetTranslate(vcpuFd uintptr, vaddr uint64) (*Translate, error) {
	var (
		kvmTranslate = kvm.IIOWR(0x85, 3*8)
		t            = &Translate{LinearAddress: vaddr}
	)

	if _, err := kvm.Ioctl(vcpuFd, kvmTranslate, uintptr(unsafe.Pointer(t))); err != nil {
		return t, fmt.Errorf("translate %#x:%w", vaddr, err)
	}

	return t, nil
}

// CPUToFD translates a CPU number to an fd.
func (m *Machine) CPUToFD(cpu int) (uintptr, error) {
	if cpu > len(m.vcpuFds) {
		return 0, fmt.Errorf("cpu %d out of range 0-%d:%w", cpu, len(m.vcpuFds), ErrBadCPU)
	}

	return m.vcpuFds[cpu], nil
}

// VtoP returns the physical address for a vCPU virtual address.
func (m *Machine) VtoP(cpu int, vaddr uintptr) (int64, error) {
	fd, err := m.CPUToFD(cpu)
	if err != nil {
		return 0, err
	}

	t, err := GetTranslate(fd, uint64(vaddr))
	if err != nil {
		return -1, err
	}

	// There can exist a valid translation for memory that does not exist.
	// For now, we call that an error.
	if t.Valid == 0 || t.PhysicalAddress > uint64(len(m.mem)) {
		return -1, fmt.Errorf("%#x:valid not set:%w", vaddr, ErrBadVA)
	}

	return int64(t.PhysicalAddress), nil
}

// GetReg gets a pointer to a register in kvm.Regs, given
// a register number from reg. This used to be a comprehensive
// case, but golangci-lint disliked the cyclomatic complexity
// So we only show the few registers we support.
func GetReg(r *kvm.Regs, reg x86asm.Reg) (*uint64, error) {
	if reg == x86asm.RAX {
		return &r.RAX, nil
	}

	if reg == x86asm.RCX {
		return &r.RCX, nil
	}

	if reg == x86asm.RDX {
		return &r.RDX, nil
	}

	if reg == x86asm.RBX {
		return &r.RBX, nil
	}

	if reg == x86asm.RSP {
		return &r.RSP, nil
	}

	if reg == x86asm.RBP {
		return &r.RBP, nil
	}

	if reg == x86asm.RSI {
		return &r.RSI, nil
	}

	if reg == x86asm.RDI {
		return &r.RDI, nil
	}

	if reg == x86asm.R8 {
		return &r.R8, nil
	}

	if reg == x86asm.R9 {
		return &r.R9, nil
	}

	if reg == x86asm.R10 {
		return &r.R10, nil
	}

	if reg == x86asm.R11 {
		return &r.R11, nil
	}

	if reg == x86asm.R12 {
		return &r.R12, nil
	}

	if reg == x86asm.R13 {
		return &r.R13, nil
	}

	if reg == x86asm.R14 {
		return &r.R14, nil
	}

	if reg == x86asm.R15 {
		return &r.R15, nil
	}

	if reg == x86asm.RIP {
		return &r.RIP, nil
	}

	return nil, fmt.Errorf("register %v%w", reg, ErrUnsupported)
}
