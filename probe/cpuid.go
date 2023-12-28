package probe

import (
	"fmt"
	"os"

	gokvmCPUID "github.com/bobuhiro11/gokvm/cpuid"
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

	printCPUID(cpuid)

	return nil
}

func printCPUID(cpuid kvm.CPUID) {
	for i := 0; i < int(cpuid.Nent); i++ {
		switch cpuid.Entries[i].Function {
		case 1:
			fmt.Printf("F_1_Edx.\n")
			printFeatures(gokvmCPUID.All_F_1_Edx, cpuid.Entries[i].Edx)
		case 7:
			if cpuid.Entries[i].Index == 0 {
				fmt.Printf("F_7_0_Edx.\n")
				printFeatures(gokvmCPUID.All_F_7_0_Edx, cpuid.Entries[i].Edx)
			}
		}
	}
}

func printFeatures[T gokvmCPUID.Feature](features []T, reg uint32) {
	enabled := []T{}
	disabled := []T{}

	for i := 0; i < len(features); i++ {
		if reg&(1<<uint(features[i])) != 0 {
			enabled = append(enabled, features[i])
		} else {
			disabled = append(disabled, features[i])
		}
	}

	fmt.Printf("* Enabled:")

	for i := 0; i < len(enabled); i++ {
		fmt.Printf(" %s", enabled[i].String())
	}

	fmt.Printf("\n* Disabled:")

	for i := 0; i < len(disabled); i++ {
		fmt.Printf(" %s", disabled[i].String())
	}

	fmt.Printf("\n\n")
}
