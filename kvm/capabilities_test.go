package kvm_test

import (
	"testing"

	"github.com/bobuhiro11/gokvm/kvm"
)

func TestCapabilityStringer(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		value kvm.Capability
		want  string
	}{
		{
			name:  "SuccessBelow5",
			value: kvm.CapIRQChip,
			want:  "CapIRQChip",
		},
		{
			name:  "SuccessAbove4Below17",
			value: kvm.CapMPState,
			want:  "CapMPState",
		},
		{
			name:  "Success18",
			value: kvm.CapIOMMU,
			want:  "CapIOMMU",
		},
		{
			name:  "SuccessBelow27",
			value: kvm.CapIRQRouting,
			want:  "CapIRQRouting",
		},
		{
			name:  "SuccessRest",
			value: kvm.CapKVMClockCtrl,
			want:  "CapKVMClockCtrl",
		},
		{
			name:  "FailTest",
			value: kvm.Capability(255),
			want:  "Capability(255)",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.value.String() != test.want {
				t.Errorf("have: %s, want: %s", test.value.String(), test.want)
			}
		})
	}
}
