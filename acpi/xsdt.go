package acpi

import (
	"bytes"
	"encoding/binary"
)

type XSDT struct {
	Header
	Entries []uint64
}

func NewXSDT(oemid, oemtableid, creatorid string) XSDT {
	h := newHeader(SigXSDT, 36, 1, oemid, oemtableid)

	return XSDT{Header: h}
}

func (x *XSDT) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, x.Header); err != nil {
		return nil, err
	}

	for _, addr := range x.Entries {
		if err := binary.Write(&buf, binary.LittleEndian, addr); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (x *XSDT) AddEntry(entry uint64) {
	x.Entries = append(x.Entries, entry)
}

func (x *XSDT) Checksum() error {
	x.Header.Checksum = 0

	data, err := x.ToBytes()
	if err != nil {
		return err
	}

	cks := uint8(0)

	for _, b := range data {
		cks += b
	}

	x.Header.Checksum = cks

	return nil
}
