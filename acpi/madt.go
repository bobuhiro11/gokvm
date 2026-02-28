package acpi

import (
	"bytes"
	"encoding/binary"
)

const (
	TypeLocalAPIC uint8 = 0 + iota
	TypeIOAPIC
	TypeInterruptSourceOverride
)

type APIC interface {
	Len() uint8
	ToBytes() ([]byte, error)
}

type LocalAPIC struct {
	Type        uint8
	Length      uint8
	ProcessorID uint8
	APICId      uint8
	Flags       uint32
}

func (l *LocalAPIC) Len() uint8 {
	return l.Length
}

func (l *LocalAPIC) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, l); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type IOAPIC struct {
	Type        uint8
	Length      uint8
	IOAPICID    uint8
	_           uint8
	APICAddress uint32
	GSIBase     uint32
}

func (i *IOAPIC) Len() uint8 {
	return i.Length
}

func (i *IOAPIC) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, i); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type InterruptSourceOverride struct {
	Type   uint8
	Length uint8
	Bus    uint8
	Source uint8
	GSI    uint32
	Flags  uint16
}

func (i *InterruptSourceOverride) Len() uint8 {
	return i.Length
}

func (i *InterruptSourceOverride) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, i); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type MADT struct {
	Header
	APICS []APIC
}

func (m *MADT) AddAPIC(apic APIC) {
	m.APICS = append(m.APICS, apic)
}

func (m *MADT) ToBytes() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, m.Header); err != nil {
		return nil, err
	}

	for _, apic := range m.APICS {
		data, err := apic.ToBytes()
		if err != nil {
			return nil, err
		}

		if _, err := buf.Write(data); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
