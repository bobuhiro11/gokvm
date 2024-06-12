package acpi

import (
	"bytes"
	"encoding/binary"
)

type DSDT struct {
	Header
	*AML
}

func NewDSDT(oemid, oemtableid string) DSDT {
	h := newHeader(SigDSDT, 36, 6, oemid, oemtableid)
	a := NewAML()

	return DSDT{h, a}
}

func (d *DSDT) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, d.Header); err != nil {
		return nil, err
	}

	if _, err := buf.Write(d.AML.ToBytes()); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
