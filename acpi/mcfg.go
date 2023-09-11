package acpi

import (
	"bytes"
	"encoding/binary"
)

type PCISegment struct {
	BaseAddress uint64
	Segment     uint16
	Start       uint8
	End         uint8
	_           uint32
}

func (p *PCISegment) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, p); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type MCFG struct {
	Header
	_        [8]byte
	Segments []PCISegment
}

func NewMCFG(oemid, oemtableid, creatorid string) MCFG {
	h := newHeader(SigMCFG, 36, 1, oemid, oemtableid)

	return MCFG{Header: h}
}

func (m *MCFG) AddSegment(seg PCISegment) {
	m.Segments = append(m.Segments, seg)
}

func (m *MCFG) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, m.Header); err != nil {
		return nil, err
	}

	for _, seg := range m.Segments {
		data, err := seg.ToBytes()
		if err != nil {
			return nil, err
		}

		if _, err := buf.Write(data); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
