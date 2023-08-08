package cpuid

import (
	"errors"
	"math/bits"

	"github.com/bobuhiro11/gokvm/kvm"
)

func cpuid_low(arg1, arg2 uint32) (eax, ebx, ecx, edx uint32) // implemented in cpuid.s

func CPUID(leaf uint32) (uint32, uint32, uint32, uint32) {
	return cpuid_low(leaf, 0)
}

type CPUIDPatch struct {
	Function uint32
	Index    uint32
	Flags    uint32
	EAXBit   uint8
	EBXBit   uint8
	ECXBit   uint8
	EDXBit   uint8
}

var errInvalidPatchset = errors.New("invalid patch. Only 1 bit allowed")

// patchCPUID patches CPUIDs before vcpu generation.
func Patch(ids *kvm.CPUID, patches []*CPUIDPatch) error {
	for _, id := range ids.Entries {
		for _, patch := range patches {
			if bits.OnesCount8(patch.EAXBit)+
				bits.OnesCount8(patch.EBXBit)+
				bits.OnesCount8(patch.ECXBit)+
				bits.OnesCount8(patch.EDXBit)+
				bits.OnesCount32(patch.Flags) != 1 {
				return errInvalidPatchset
			}

			if id.Function == patch.Function && id.Index == patch.Index {
				id.Flags |= 1 << patch.Flags
				id.Eax |= 1 << patch.EAXBit
				id.Ebx |= 1 << patch.EBXBit
				id.Ecx |= 1 << patch.ECXBit
				id.Edx |= 1 << patch.EDXBit
			}
		}
	}

	return nil
}
