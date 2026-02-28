package acpi_test

import (
	"bytes"
	"testing"

	"github.com/bobuhiro11/gokvm/acpi"
)

func TestCalcPkgLength(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		size uint32
		exp  []byte
		err  error
	}{
		{
			name: "1ByteSize",
			size: 62,
			exp:  []byte{63},
		},
		{
			name: "2ByteSize",
			size: 64,
			exp:  []byte{1<<6 | (66 & 0xf), 66 >> 4},
		},
		{
			name: "3ByteSize",
			size: 4096,
			exp:  []byte{2<<6 | (4099 & 0xf), 0, 1},
		},
		{
			name: "4ByteSize",
			size: 536870912,
			exp:  []byte{3<<6 | (536870916 & 0xf), 0, 0, 0},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val := acpi.CalcPkgLength(tt.size, true)
			if !bytes.Equal(val, tt.exp) {
				t.Fatalf("byte not match. Have: 0x%x, want: 0x%x", val, tt.exp)
			}
		})
	}
}
