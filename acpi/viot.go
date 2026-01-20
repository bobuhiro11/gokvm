package acpi

import (
	"bytes"
	"encoding/binary"
)

type VIOTNode interface {
	ToBytes() ([]byte, error)
}

type ViotVirtualPCINode struct {
	Type         uint8
	_            uint8
	Length       uint16
	PCISegment   uint16
	PCIBDFNumber uint16
	_            uint64
}

func (v *ViotVirtualPCINode) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	return buf.Bytes(), nil
}

type ViotPCIRangeNode struct {
	Type            uint8
	_               uint8
	Length          uint16
	EndpointStart   uint32
	PCISegmentStart uint16
	PCISegmentEnd   uint16
	PCIBDFStart     uint16
	PCIBDFEnd       uint16
	OutputNode      uint16
	_               uint64
}

func (v *ViotPCIRangeNode) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	return buf.Bytes(), nil
}

type VIOT struct {
	Header
	Nodes []VIOTNode
}

func NewVIOT(oemid, oemtableid, creatorid string) VIOT {
	h := newHeader(SigVIOT, 36, 1, oemid, oemtableid)

	return VIOT{Header: h}
}

func (v *VIOT) AddNode(node VIOTNode) {
	v.Nodes = append(v.Nodes, node)
}

func (v *VIOT) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, v.Header); err != nil {
		return nil, err
	}

	for _, node := range v.Nodes {
		data, err := node.ToBytes()
		if err != nil {
			return nil, err
		}

		if _, err := buf.Write(data); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
