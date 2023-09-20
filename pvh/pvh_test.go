package pvh_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/pvh"
)

func TestGdtEntry(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		flag       uint16
		base       uint32
		limit      uint32
		expEntry   uint64
		tableIndex uint8
		expSeg     kvm.Segment
	}{
		{
			name:       "Zero Entry",
			flag:       0,
			base:       0,
			limit:      0,
			expEntry:   0,
			tableIndex: 0,
			expSeg: kvm.Segment{
				Base:     0,
				Limit:    0,
				Selector: 0,
				Typ:      0,
				Present:  0,
				DPL:      0,
				DB:       0,
				S:        0,
				L:        0,
				G:        0,
				AVL:      0,
				Unusable: 1,
			},
		},
		{
			name:       "Code Segment Entry",
			flag:       0xc09b,
			base:       0,
			limit:      0xffffffff,
			expEntry:   0xcf9b000000ffff,
			tableIndex: 1,
			expSeg: kvm.Segment{
				Base:     0,
				Limit:    0xffffffff,
				Selector: 0x8,
				Typ:      0xB,
				Present:  0x1,
				DPL:      0x0,
				DB:       0x1,
				S:        0x1,
				L:        0x0,
				G:        0x1,
				AVL:      0x0,
				Unusable: 0x0,
			},
		},
		{
			name:       "Data Segment Entry",
			flag:       0xc093,
			base:       0,
			limit:      0xffffffff,
			expEntry:   0xcf93000000ffff,
			tableIndex: 2,
			expSeg: kvm.Segment{
				Base:     0,
				Limit:    0xffffffff,
				Selector: 0x10,
				Typ:      0x3,
				Present:  0x1,
				DPL:      0x0,
				DB:       0x1,
				S:        0x1,
				L:        0x0,
				G:        0x1,
				AVL:      0x0,
				Unusable: 0x0,
			},
		},
		{
			name:       "TSS Segment Entry",
			flag:       0x008b,
			base:       0,
			limit:      0x67,
			expEntry:   0x8b0000000067,
			tableIndex: 3,
			expSeg: kvm.Segment{
				Base:     0,
				Limit:    0x67,
				Selector: 0x18,
				Typ:      0xB,
				Present:  0x1,
				DPL:      0x0,
				DB:       0x0,
				S:        0x0,
				L:        0x0,
				G:        0x0,
				AVL:      0x0,
				Unusable: 0x0,
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := pvh.GdtEntry(tt.flag, tt.base, tt.limit)
			if tt.expEntry != res {
				t.Fatalf("Test %s failed: got: 0x%x, exp: 0x%x", tt.name, res, tt.expEntry)
			}
		})

		t.Run(tt.name, func(t *testing.T) {
			seg := pvh.SegmentFromGDT(tt.expEntry, tt.tableIndex)
			var buf, expbuf bytes.Buffer

			if err := binary.Write(&buf, binary.LittleEndian, seg); err != nil {
				t.Fatal(err)
			}

			if err := binary.Write(&expbuf, binary.LittleEndian, tt.expSeg); err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf.Bytes(), expbuf.Bytes()) {
				t.Fatalf("Test %s failed: got: %x, exp: %x", tt.name, seg, tt.expSeg)
			}
		})

		t.Run(tt.name, func(t *testing.T) {
			gdt := pvh.CreateGDT()
			if gdt[tt.tableIndex] != tt.expEntry {
				t.Fatalf("Test %s failed: got: 0x%x, exp: 0x%x", tt.name, gdt[tt.tableIndex], tt.expEntry)
			}
		})
	}
}
