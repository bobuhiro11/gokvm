package ebda

import (
	"bytes"
	"encoding/binary"

	"github.com/bobuhiro11/gokvm/bootparam"
)

// Extended BIOS Data Area (EBDA).
type EBDA struct {
	// padding
	// It must be aligned with 16 bytes and its size must be less than 1KB.
	// https://github.com/torvalds/linux/blob/2f111a6fd5b5297b4e92f53798ca086f7c7d33a4/arch/x86/kernel/mpparse.c#L597
	_        [16 * 3]uint8
	mpfIntel MPFIntel
	mpcTable MPCTable
}

func (e *EBDA) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, e); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func New() (*EBDA, error) {
	e := &EBDA{}

	mpfIntel, err := NewMPFIntel()
	if err != nil {
		return e, err
	}

	e.mpfIntel = *mpfIntel

	mpcTable, err := NewMPCTable()
	if err != nil {
		return e, err
	}

	e.mpcTable = *mpcTable

	return e, nil
}

// Intel MP Floating Pointer Structure
// ported from https://github.com/torvalds/linux/blob/5bfc75d92/arch/x86/include/asm/mpspec_def.h#L22-L33
type MPFIntel struct {
	Signature     uint32
	PhysPtr       uint32
	Length        uint8
	Specification uint8
	CheckSum      uint8
	Feature1      uint8
	Feature2      uint8
	Feature3      uint8
	Feature4      uint8
	Feature5      uint8
}

func NewMPFIntel() (*MPFIntel, error) {
	m := &MPFIntel{}
	m.Signature = (('_' << 24) | ('P' << 16) | ('M' << 8) | '_')
	m.Length = 1 // this must be 1
	m.Specification = 4
	m.PhysPtr = bootparam.EBDAStart + 0x40

	var err error

	m.CheckSum, err = m.CalcCheckSum()
	if err != nil {
		return m, err
	}

	m.CheckSum ^= uint8(0xff)
	m.CheckSum++

	return m, nil
}

func (m *MPFIntel) CalcCheckSum() (uint8, error) {
	bytes, err := m.Bytes()
	if err != nil {
		return 0, err
	}

	tmp := uint32(0)
	for _, b := range bytes {
		tmp += uint32(b)
	}

	return uint8(tmp & 0xff), nil
}

func (m *MPFIntel) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, m); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

// MP Configuration Table Header
// ported from https://github.com/torvalds/linux/blob/5bfc75d92/arch/x86/include/asm/mpspec_def.h#L37-L49
type MPCTable struct {
	Signature uint32
	Length    uint16
	Spec      uint8
	CheckSum  uint8
	OEM       [8]uint8
	ProductID [12]uint8
	OEMPtr    uint32
	OEMSize   uint32
	OEMCount  uint32
	LAPIC     uint32 // Local APIC addresss must be set.
	Reserved  uint32
}

func NewMPCTable() (*MPCTable, error) {
	m := &MPCTable{}
	m.Signature = (('P' << 24) | ('M' << 16) | ('C' << 8) | 'P')
	m.Length = 42 // this field must contain the size of entries.
	m.Spec = 1

	var err error

	m.CheckSum, err = m.CalcCheckSum()
	if err != nil {
		return m, err
	}

	m.CheckSum ^= uint8(0xff)
	m.CheckSum++

	return m, nil
}

func (m *MPCTable) CalcCheckSum() (uint8, error) {
	bytes, err := m.Bytes()
	if err != nil {
		return 0, err
	}

	tmp := uint32(0)
	for _, b := range bytes {
		tmp += uint32(b)
	}

	return uint8(tmp & 0xff), nil
}

func (m *MPCTable) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, m); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}
