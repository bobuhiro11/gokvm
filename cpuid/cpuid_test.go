package cpuid_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/cpuid"
)

func TestCPUID(t *testing.T) {
	t.Parallel()

	eax, ebx, ecx, edx := cpuid.CPUID(0)

	t.Logf("eax:0x%x ebx:0x%x ecx:0x%x edx:0x%x",
		eax, ebx, ecx, edx)

	s := []rune{}
	for _, x := range []uint32{ebx, edx, ecx} {
		s = append(s, rune(x>>0)&0xff)
		s = append(s, rune(x>>8)&0xff)
		s = append(s, rune(x>>16)&0xff)
		s = append(s, rune(x>>24)&0xff)
	}

	if string(s) != "GenuineIntel" && string(s) != "AuthenticAMD" {
		t.Fatalf("Unknown CPU vender found: %s", string(s))
	}
}
