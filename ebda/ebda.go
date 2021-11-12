package ebda

import (
	"bytes"
	"encoding/binary"
	"unsafe"

	"github.com/bobuhiro11/gokvm/bootparam"
)

// When we started working on the SMP implementation, we fixed it to 2 for
// simplicity, and it should match the CLI arguments.
const NumCPUs = 2

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
	OEMSize   uint16
	OEMCount  uint16
	LAPIC     uint32 // Local APIC addresss must be set.
	Reserved  uint32

	mpcCPU    [NumCPUs]MPCCpu
	mpcBus    [2]MPCBus
	mpcIntSrc [2]MPCIntSrc
	mpcIOAPIC MPCIOAPIC
}

const (
	IOAPICDefaultPhysBase = 0xfec00000
	APICDefaultPhysBase   = 0xfee00000
	APICBaseAddrStep      = 0x00400000
	IOAPICBaseAddrStep    = 0x00100000
	KVMAPICVersion        = 0x14
	PCIBusID              = 0
	ISABusID              = 1
)

func apicAddr(apic uint32) uint32 {
	return APICDefaultPhysBase + apic*APICBaseAddrStep
}

func ioApicAddr(ioapic uint32) uint32 {
	return IOAPICDefaultPhysBase + ioapic*IOAPICBaseAddrStep
}

func NewMPCTable() (*MPCTable, error) {
	m := &MPCTable{}
	m.Signature = (('P' << 24) | ('M' << 16) | ('C' << 8) | 'P')
	m.Length = uint16(unsafe.Sizeof(MPCTable{})) // this field must contain the size of entries.
	m.Spec = 4
	m.LAPIC = apicAddr(0)
	m.OEMCount = 7 // This must be the number of entries

	var err error

	for i := 0; i < NumCPUs; i++ {
		mpcCPU, err := NewMPCCpu(i)
		if err != nil {
			return m, err
		}

		m.mpcCPU[i] = *mpcCPU
	}

	mpcBus, _ := NewMPCBus(true)
	m.mpcBus[0] = *mpcBus
	mpcBus, _ = NewMPCBus(false)
	m.mpcBus[1] = *mpcBus

	mpcIntSrc, _ := NewMPCIntSrc(true)
	m.mpcIntSrc[0] = *mpcIntSrc
	mpcIntSrc, _ = NewMPCIntSrc(false)
	m.mpcIntSrc[1] = *mpcIntSrc

	mpcIOAPIC, _ := NewMPCIOAPIC()
	m.mpcIOAPIC = *mpcIOAPIC

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

type MPCCpu struct {
	Type        uint8
	APICID      uint8 // Local APIC number
	APICVer     uint8
	CPUFlag     uint8
	CPUFeature  uint32
	FeatureFlag uint32
	Reserved    [2]uint32
}

func NewMPCCpu(i int) (*MPCCpu, error) {
	m := &MPCCpu{}

	m.Type = 0
	m.APICID = uint8(i)
	m.APICVer = KVMAPICVersion
	m.CPUFlag |= 1 // enabled processor

	if i == 0 {
		m.CPUFlag |= 2 // boot processor
	}
	// refs: commit 0c7c14a747e9eb2c3cacef60fb74b0698c9d3adf in kvmtool
	m.CPUFeature = 0x600  // STEPPING
	m.FeatureFlag = 0x201 // CPU_FEATURE_APIC

	return m, nil
}

type MPCBus struct {
	Type    uint8
	BusID   uint8
	BusType [6]uint8
}

func NewMPCBus(isPCIBus bool) (*MPCBus, error) {
	m := &MPCBus{}

	m.Type = 1 // bus

	if isPCIBus {
		m.BusID = PCIBusID
		m.BusType[0] = 'P'
		m.BusType[1] = 'C'
		m.BusType[2] = 'I'
	} else {
		m.BusID = ISABusID
		m.BusType[0] = 'I'
		m.BusType[1] = 'S'
		m.BusType[2] = 'A'
	}

	m.BusType[3] = ' '
	m.BusType[4] = ' '
	m.BusType[5] = ' '

	return m, nil
}

type MPCIntSrc struct {
	Type      uint8
	IrqType   uint8
	IrqFlag   uint16
	SrcBus    uint8
	SrcBusIrq uint8
	DstAPIC   uint8
	DstIrq    uint8
}

const (
	MPINT    = 0
	MPNMI    = 1
	MPSMI    = 2
	MPExtINT = 3

	MPIrqDirDefault = 0
)

func NewMPCIntSrc(isLINT0 bool) (*MPCIntSrc, error) {
	m := &MPCIntSrc{}
	m.Type = 4 // MP_LINTSRC

	if isLINT0 {
		m.IrqType = MPINT
		m.IrqFlag = MPIrqDirDefault
		m.SrcBus = ISABusID
		m.SrcBusIrq = 0
		m.DstAPIC = 0
		m.DstIrq = 0
	} else {
		m.IrqType = MPNMI
		m.IrqFlag = MPIrqDirDefault
		m.SrcBus = ISABusID
		m.SrcBusIrq = 0
		m.DstAPIC = 0
		m.DstIrq = 1
	}

	return m, nil
}

type MPCIOAPIC struct {
	Type     uint8
	APICID   uint8
	APICVer  uint8
	Flags    uint8
	APICAddr uint32
}

func NewMPCIOAPIC() (*MPCIOAPIC, error) {
	m := &MPCIOAPIC{}
	m.Type = 2
	m.APICID = 3 // nr_vcpu + 1
	m.APICVer = KVMAPICVersion
	m.Flags = 0x01 // MPC_APIC_USABLE 0x01
	m.APICAddr = ioApicAddr(0)

	return m, nil
}
