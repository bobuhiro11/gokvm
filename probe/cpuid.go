package probe

import (
	"fmt"
	"os"

	"github.com/bobuhiro11/gokvm/kvm"
)

// CPUID call 'KVM_GET_SUPPORTED_CPUID' and print the result.
func CPUID() error {
	kvmFile, err := os.Open("/dev/kvm")
	if err != nil {
		return err
	}
	defer kvmFile.Close()

	kvmfd := kvmFile.Fd()

	cpuid := kvm.CPUID{
		Nent:    100,
		Entries: make([]kvm.CPUIDEntry2, 100),
	}

	if err := kvm.GetSupportedCPUID(kvmfd, &cpuid); err != nil {
		return err
	}

	for _, e := range cpuid.Entries {
		fmt.Printf("0x%08x 0x%02x: eax=0x%08x ebx=0x%08x ecx=0x%08x edx=0x%08x (flag:%x)\n",
			e.Function, e.Index, e.Eax, e.Ebx, e.Ecx, e.Edx, e.Flags)
	}

	return nil
}
