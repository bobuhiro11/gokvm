//nolint:dupl,paralleltest
package kvm_test

import (
	"errors"
	"math"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/bobuhiro11/gokvm/kvm"
)

func TestGetAPIVersion(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	_, err = kvm.GetAPIVersion(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateVM(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetTSSAddr(vmFd, 0xffffd000); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetIdentityMapAddr(vmFd, 0xffffc000); err != nil {
		t.Fatal(err)
	}

	_, err = kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCPUID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	CPUID := kvm.CPUID{
		Nent:    100,
		Entries: make([]kvm.CPUIDEntry2, 100),
	}

	if err := kvm.GetSupportedCPUID(devKVM.Fd(), &CPUID); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(CPUID.Entries); i++ {
		CPUID.Entries[i].Eax = kvm.CPUIDFeatures
		CPUID.Entries[i].Ebx = 0x4b4d564b // KVMK
		CPUID.Entries[i].Ecx = 0x564b4d56 // VMKV
		CPUID.Entries[i].Edx = 0x4d       // M
	}

	if err := kvm.SetCPUID2(vcpuFd, &CPUID); err != nil {
		t.Fatal(err)
	}

	if err := kvm.GetCPUID2(vcpuFd, &CPUID); err != nil {
		t.Fatal(err)
	}
}

func TestCreateVCPU(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	err = kvm.CreateIRQChip(vmFd)
	if err != nil {
		t.Fatal(err)
	}

	err = kvm.CreatePIT2(vmFd)
	if err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	sregs, err := kvm.GetSregs(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	err = kvm.SetSregs(vcpuFd, sregs)
	if err != nil {
		t.Fatal(err)
	}

	regs, err := kvm.GetRegs(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	err = kvm.SetRegs(vcpuFd, regs)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetVCPUMMapSize(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	_, err = kvm.GetVCPUMMmapSize(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateVCPUWithNoVmFd(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = kvm.CreateVCPU(devKVM.Fd(), 0)
	if err == nil {
		t.Fatal(err)
	}
}

// mirror from https://lwn.net/Articles/658512/
func TestAddNum(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	mem, err := syscall.Mmap(-1, 0, 0x1000, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		t.Fatal(err)
	}

	code := []byte{0xba, 0xf8, 0x03, 0x00, 0xd8, 0x04, '0', 0xee, 0xb0, '\n', 0xee, 0xf4}
	copy(mem, code)

	if err = kvm.SetUserMemoryRegion(vmFd, &kvm.UserspaceMemoryRegion{
		Slot:          0,
		Flags:         0,
		GuestPhysAddr: 0x1000,
		MemorySize:    0x1000,
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&mem[0]))),
	}); err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	mmapSize, err := kvm.GetVCPUMMmapSize(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	r, err := syscall.Mmap(int(vcpuFd), 0, int(mmapSize), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}

	run := (*kvm.RunData)(unsafe.Pointer(&r[0]))

	sregs, err := kvm.GetSregs(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	sregs.CS.Base, sregs.CS.Selector = 0, 0

	if err = kvm.SetSregs(vcpuFd, sregs); err != nil {
		t.Fatal(err)
	}

	if err = kvm.SetRegs(vcpuFd, &kvm.Regs{
		RIP: 0x1000, RAX: 2, RBX: 2, RFLAGS: 0x2, RCX: 0, RDX: 0, RSI: 0,
		RDI: 0, RSP: 0, RBP: 0, R8: 0, R9: 0, R10: 0, R11: 0, R12: 0,
		R13: 0, R14: 0, R15: 0,
	}); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SingleStep(vcpuFd, true); err != nil {
		t.Logf("kvm.SingleStep(%d, true): got %v, want nil", vcpuFd, err)
	}

	if err := kvm.SingleStep(vcpuFd, false); err != nil {
		t.Logf("kvm.SingleStep(%d, false): got %v, want nil", vcpuFd, err)
	}

	if err := kvm.SingleStep(uintptr(math.MaxUint), false); !errors.Is(err, syscall.EBADF) {
		t.Errorf("fixme:kvm.SingleStep(%d, false): got %v, want %v", vcpuFd, err, syscall.EBADF)
	}

	if err := kvm.SingleStep(vcpuFd, true); err != nil {
		t.Logf("kvm.SingleStep(%d, true): got %v, want nil", vcpuFd, err)
	}

	// While we are running the test, run it with SingleStep enabled,
	// so we can test that too.
	var singleStepOK bool

	for {
		if err = kvm.Run(vcpuFd); err != nil {
			t.Logf("kvm.Run(%d) returns with %v", vcpuFd, err)
		}

		switch kvm.ExitType(run.ExitReason) {
		case kvm.EXITHLT:
			if !singleStepOK {
				t.Errorf("singleStepOK: got false, want true; single step is not working")
			}

			return

		case kvm.EXITIO:
			direction, size, port, count, offset := run.IO()
			if direction == uint64(kvm.EXITIOOUT) && size == 1 && port == 0x3f8 && count == 1 {
				p := uintptr(unsafe.Pointer(run))
				c := *(*byte)(unsafe.Pointer(p + uintptr(offset)))
				// t.Logf("output from IO port: \"%c\"\n", c)

				if c != '4' && c != '\n' {
					t.Fatal("Unexpected Output")
				}
			} else {
				t.Fatal("Unexpected KVM_EXIT_IO")
			}
		case kvm.EXITDEBUG:
			singleStepOK = true
		case kvm.EXITDCR,
			kvm.EXITINTR,
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
			kvm.EXITTPRACCESS,
			kvm.EXITUNKNOWN:
			t.Fatalf("Unexpected EXIT REASON = %s\n", kvm.ExitType(run.ExitReason).String())
		default:
			t.Fatalf("Unexpected EXIT REASON = %s\n", kvm.ExitType(run.ExitReason).String())
		}
	}
}

func TestSetMemLogDirtyPages(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	u := kvm.UserspaceMemoryRegion{}
	u.SetMemLogDirtyPages()
	u.SetMemReadonly()

	if u.Flags != 0x3 {
		t.Fatal("unexpected flags")
	}
}

func TestIRQLine(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.CreateIRQChip(vmFd); err != nil {
		t.Fatal(err)
	}

	if err := kvm.IRQLineStatus(vmFd, 4, 0); err != nil {
		t.Fatal(err)
	}
}

func TestIoctlStringer(t *testing.T) {
	for _, test := range []struct {
		name string
		val  kvm.ExitType
		want string
	}{
		{name: "First error", val: kvm.EXITUNKNOWN, want: "EXITUNKNOWN"},
		{name: "Middle error", val: kvm.EXITIO, want: "EXITIO"},
		{name: "Last error", val: kvm.EXITINTERNALERROR, want: "EXITINTERNALERROR"},
		{name: "Out of range error", val: kvm.ExitType(1024), want: "ExitType(1024)"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got := test.val.String()
			if got != test.want {
				t.Errorf("%s:%s != %s", test.name, test.want, got)
			}
		})
	}
}

func TestGetSetPID2(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.CreateIRQChip(vmFd); err != nil {
		t.Fatal(err)
	}

	if err := kvm.CreatePIT2(vmFd); err != nil {
		t.Fatal(err)
	}

	pstate := &kvm.PITState2{}

	if err := kvm.GetPIT2(vmFd, pstate); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetPIT2(vmFd, pstate); err != nil {
		t.Fatal(err)
	}
}

// func TestSetGSIRouting(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.CreateIRQChip(vmFd); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	irqR := &kvm.IRQRouting{
// 		Nr:      0,
// 		Flags:   0,
// 		Entries: make([]kvm.IRQRoutingEntry, 1),
// 	}
// 
// 	if err := kvm.SetGSIRouting(vmFd, irqR); err != nil {
// 		t.Fatal(err)
// 	}
// }

func TestCoalescedMMIO(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.RegisterCoalescedMMIO(vmFd, 0xFFFE000, 0x1000); err != nil {
		t.Fatal(err)
	}

	if err := kvm.UnregisterCoalescedMMIO(vmFd, 0xFFFE000, 0x1000); err != nil {
		t.Fatal(err)
	}
}

func TestSetNrMMUPages(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetNrMMUPages(vmFd, 0x1000); err != nil {
		t.Fatal(err)
	}

	retval := uint64(0)

	if err := kvm.GetNrMMUPages(vmFd, &retval); err != nil {
		t.Fatal(err)
	}
}

func TestGetDirtyLog(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	mem, err := syscall.Mmap(-1, 0, 0x1000, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		t.Fatal(err)
	}

	if err = kvm.SetUserMemoryRegion(vmFd, &kvm.UserspaceMemoryRegion{
		Slot:          0,
		Flags:         0,
		GuestPhysAddr: 0x1000,
		MemorySize:    0x1000,
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&mem[0]))),
	}); err != nil {
		t.Fatal(err)
	}

	dl := &kvm.DirtyLog{
		Slot:   0,
		BitMap: 0,
	}

	if err := kvm.GetDirtyLog(vmFd, dl); err != nil {
		t.Fatal(err)
	}
}

func TestSetGetIRQChip(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.CreateIRQChip(vmFd); err != nil {
		t.Fatal(err)
	}

	irqc := &kvm.IRQChip{
		ChipID: 0,
		Chip:   [512]byte{},
	}

	if err := kvm.GetIRQChip(vmFd, irqc); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetIRQChip(vmFd, irqc); err != nil {
		t.Fatal(err)
	}
}

func TestGetEmulatedCPUID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	kvmCPUID := &kvm.CPUID{
		Nent:    100,
		Entries: make([]kvm.CPUIDEntry2, 100),
	}

	if err := kvm.GetEmulatedCPUID(devKVM.Fd(), kvmCPUID); err != nil {
		t.Fatal(err)
	}
}

func TestSetGetTSCKHz(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	freq, err := kvm.GetTSCKHz(vcpuFd)
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetTSCKHz(vcpuFd, freq); err != nil {
		t.Fatal(err)
	}
}

func TestSetGetClock(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	cd := &kvm.ClockData{}

	if err := kvm.GetClock(vmFd, cd); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetClock(vmFd, cd); err != nil {
		t.Fatal(err)
	}
}

func TestCreateDev(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	dev := &kvm.Device{
		Type:  uint32(kvm.DevVFIO),
		Fd:    0,
		Flags: 1,
	}

	for i := 0; i <= int(kvm.DevMAX); i++ {
		if err = kvm.CreateDev(vmFd, dev); err != nil {
			if !errors.Is(err, syscall.ENODEV) {
				t.Fatal(err)
			}
		}
	}
}

// func TestInjectInterrpt(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	// Pass an invalid value, because the vm is empty and error out for every other error
// 	if err := kvm.InjectInterrupt(vcpuFd, 0xFFF0); !errors.Is(err, syscall.EFAULT) {
// 		t.Fatal(err)
// 	}
// }

func TestGetMSRIndexList(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapGETMSRFeatures)
	if err != nil {
		t.Fatal(err)
	}

	if int(ret) <= 0 {
		t.Skipf("Skipping test since CapGETMSRFeatures is disable")
	}

	list := kvm.MSRList{}

	// The first time we probe the number of MSRs.
	if err := kvm.GetMSRIndexList(devKVM.Fd(), &list); !errors.Is(err, syscall.E2BIG) {
		t.Fatal(err)
	}

	// The second time we get the contents of the entries.
	if err := kvm.GetMSRIndexList(devKVM.Fd(), &list); err != nil {
		t.Fatal(err)
	}
}

func TestGetMSRFeatureIndexList(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapGETMSRFeatures)
	if err != nil {
		t.Fatal(err)
	}

	if int(ret) <= 0 {
		t.Skipf("Skipping test since CapGETMSRFeatures is disable")
	}

	list := kvm.MSRList{}

	// The first time we probe the number of MSRs.
	if err := kvm.GetMSRFeatureIndexList(devKVM.Fd(), &list); !errors.Is(err, syscall.E2BIG) {
		t.Fatal(err)
	}

	// The second time we get the contents of the entries.
	if err := kvm.GetMSRFeatureIndexList(devKVM.Fd(), &list); err != nil {
		t.Fatal(err)
	}

	var entryFound bool

	for _, msr := range list.Indicies {
		if msr != 0 {
			entryFound = true
		}
	}

	if !entryFound {
		t.Fatalf("no entry has been found and no error occurred. That's odd")
	}
}

func TestGetSetLocalAPIC(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	if err := kvm.CreateIRQChip(vmFd); err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	lapic := &kvm.LAPICState{
		Regs: [1024]byte{},
	}

	if err := kvm.SetLocalAPIC(vcpuFd, lapic); err != nil {
		t.Fatal(err)
	}

	if err := kvm.GetLocalAPIC(vcpuFd, lapic); err != nil {
		t.Fatal(err)
	}
}

// func TestReinjectControl(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.CreateIRQChip(vmFd); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.CreatePIT2(vmFd); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.ReinjectControl(vmFd, 1); err != nil {
// 		t.Fatal(err)
// 	}
// }

func TestTranslate(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	mem, err := syscall.Mmap(-1, 0, 0x1000, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
	if err != nil {
		t.Fatal(err)
	}

	if err = kvm.SetUserMemoryRegion(vmFd, &kvm.UserspaceMemoryRegion{
		Slot:          0,
		Flags:         0,
		GuestPhysAddr: 0x1000,
		MemorySize:    0x1000,
		UserspaceAddr: uint64(uintptr(unsafe.Pointer(&mem[0]))),
	}); err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Test a good address
	tOk := &kvm.Translation{
		LinearAddress: 0,
	}

	if err := kvm.Translate(vcpuFd, tOk); err != nil || tOk.PhysicalAddress != 0 {
		t.Errorf("m.VtoP(0, 0): got (%#x, %v), want 0, nil", tOk.PhysicalAddress, err)
	}
}

// func TestTRPAccessReporting(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapVAPIC)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if int(ret) <= 0 {
// 		t.Skipf("Skipping test since CapVAPIC is disable")
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ctl := &kvm.TRPAccessCtl{
// 		Enable: 1,
// 	}
// 
// 	if err := kvm.TRPAccessReporting(vcpuFd, ctl); err != nil {
// 		t.Fatal(err)
// 	}
// }
// 
// func TestGetSetMPState(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapMPState)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if int(ret) <= 0 {
// 		t.Skip("Skipping test since CapMPState is disable")
// 	}
// 
// 	mps := &kvm.MPState{
// 		State: kvm.MPStateUninitialized,
// 	}
// 
// 	if err := kvm.GetMPState(vcpuFd, mps); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.SetMPState(vcpuFd, mps); err != nil {
// 		t.Fatal(err)
// 	}
// }

// func TestX86MCE(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapMCE)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if int(ret) <= 0 {
// 		t.Skip("Skipping test since CapMCE is disable")
// 	}
// 
// 	mceCap := uint64(0x0)
// 
// 	if err := kvm.X86GetMCECapSupported(devKVM.Fd(), &mceCap); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	mceCap = 1
// 
// 	if err := kvm.X86SetupMCE(vcpuFd, &mceCap); err != nil {
// 		t.Fatal(err)
// 	}
// }

func TestGetSetVCPUEvents(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapVCPUEvents)
	if err != nil {
		t.Fatal(err)
	}

	if int(ret) <= 0 {
		t.Skip("Skipping test since CapVCPUEvents is disable")
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	event := &kvm.VCPUEvents{}

	if err := kvm.GetVCPUEvents(vcpuFd, event); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetVCPUEvents(vcpuFd, event); err != nil {
		t.Fatal(err)
	}
}

// func TestGetSetDebugRegs(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapDebugRegs)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if int(ret) <= 0 {
// 		t.Skip("Skipping test since CapDebugRegs is disable")
// 	}
// 
// 	dregs := &kvm.DebugRegs{}
// 
// 	if err := kvm.GetDebugRegs(vcpuFd, dregs); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.SetDebugRegs(vcpuFd, dregs); err != nil {
// 		t.Fatal(err)
// 	}
// }

func TestGetSetXCRS(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("Skipping test since we are not root")
	}

	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	vmFd, err := kvm.CreateVM(devKVM.Fd())
	if err != nil {
		t.Fatal(err)
	}

	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
	if err != nil {
		t.Fatal(err)
	}

	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapXCRS)
	if err != nil {
		t.Fatal(err)
	}

	if int(ret) <= 0 {
		t.Skipf("Skipping test since CapXCRS is disable")
	}

	xcrs := &kvm.XCRS{}

	if err := kvm.GetXCRS(vcpuFd, xcrs); err != nil {
		t.Fatal(err)
	}

	if err := kvm.SetXCRS(vcpuFd, xcrs); err != nil {
		t.Fatal(err)
	}
}

// func TestSMI(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapX86SMM)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if int(ret) <= 0 {
// 		t.Skipf("Skipping test since CapX86SMM is disable")
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.PutSMI(vcpuFd); err != nil {
// 		t.Fatal(err)
// 	}
// }

// func TestGetSetSRegs2(t *testing.T) {
// 	if os.Getuid() != 0 {
// 		t.Skipf("Skipping test since we are not root")
// 	}
// 
// 	devKVM, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0o644)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	defer devKVM.Close()
// 
// 	vmFd, err := kvm.CreateVM(devKVM.Fd())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	ret, err := kvm.CheckExtension(devKVM.Fd(), kvm.CapSREGS2)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if int(ret) <= 0 {
// 		t.Skipf("Skipping test since CapSREGS2 is disable")
// 	}
// 
// 	vcpuFd, err := kvm.CreateVCPU(vmFd, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	sregs2 := &kvm.SRegs2{}
// 
// 	if err := kvm.GetSRegs2(vcpuFd, sregs2); err != nil {
// 		t.Fatal(err)
// 	}
// 
// 	if err := kvm.SetSRegs2(vcpuFd, sregs2); err != nil {
// 		t.Fatal(err)
// 	}
// }
