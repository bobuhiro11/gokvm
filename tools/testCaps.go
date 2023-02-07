package tools

import (
	"fmt"
	"os"

	"github.com/bobuhiro11/gokvm/kvm"
)

// TestCaps probes the system for kvm capabilities.
func TestCaps() error {
	X86tests := []kvm.Capability{
		kvm.CapIRQChip,
		kvm.CapUserMemory,
		kvm.CapSetTSSAddr,
		kvm.CapEXTCPUID,
		kvm.CapMPState,
		kvm.CapCoalescedMMIO,
		kvm.CapUserNMI,
		kvm.CapSetGuestDebug,
		kvm.CapReinjectControl,
		kvm.CapIRQRouting,
		kvm.CapMCE,
		kvm.CapIRQFD,
		kvm.CapPIT2,
		kvm.CapSetBootCPUID,
		kvm.CapPITState2,
		kvm.CapIOEventFD,
		kvm.CapAdjustClock,
		kvm.CapVCPUEvents,
		kvm.CapINTRShadow,
		kvm.CapDebugRegs,
		kvm.CapEnableCap,
		kvm.CapXSave,
		kvm.CapXCRS,
		kvm.CapTSCControl,
		kvm.CapONEREG,
		kvm.CapKVMClockCtrl,
		kvm.CapSignalMSI,
		kvm.CapDeviceCtrl,
		kvm.CapEXTEmulCPUID,
		kvm.CapVMAttributes,
		kvm.CapX86SMM,
		kvm.CapX86DisableExits,
		kvm.CapGETMSRFeatures,
		kvm.CapNestedState,
		kvm.CapCoalescedPIO,
		kvm.CapManualDirtyLogProtect2,
		kvm.CapPMUEventFilter,
		kvm.CapX86UserSpaceMSR,
		kvm.CapX86MSRFilter,
		kvm.CapX86BusLockExit,
		kvm.CapSREGS2,
		kvm.CapBinaryStatsFD,
		kvm.CapXSave2,
		kvm.CapSysAttributes,
		kvm.CapVMTSCControl,
		kvm.CapX86TripleFaultEvent,
		kvm.CapX86NotifyVMExit,
	}

	kvmFile, err := os.Open("/dev/kvm")
	if err != nil {
		return err
	}

	kvmfd := kvmFile.Fd()

	for _, test := range X86tests {
		res, err := kvm.CheckExtension(kvmfd, test)
		if err != nil {
			return err
		}

		fmt.Printf("%-30s: %t\n", test, (res != 0))
	}

	return nil
}
