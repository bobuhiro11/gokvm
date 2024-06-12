package acpi

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
)

type AMLOp uint8

const (
	OpZero AMLOp = 0x00
	OpOne  AMLOp = 0x01

	OpName            AMLOp = 0x08
	OpBytePrefix      AMLOp = 0x0A
	OpWordPrefix      AMLOp = 0x0B
	OpDWordPrefix     AMLOp = 0x0C
	OpString          AMLOp = 0x0D
	OpQWordPrefix     AMLOp = 0x0E
	OpScope           AMLOp = 0x10
	OpBuffer          AMLOp = 0x11
	OpPackage         AMLOp = 0x12
	OpVarPackage      AMLOp = 0x13
	OpMethod          AMLOp = 0x14
	OpDualNamePrefix  AMLOp = 0x2E
	OpMultiNamePrefix AMLOp = 0x2F

	OpNameCharBase AMLOp = 0x40

	OpExtPrefix   AMLOp = 0x5b
	OpMutex       AMLOp = 0x01
	OpCreateFile  AMLOp = 0x13
	OpAcquire     AMLOp = 0x23
	OpRelease     AMLOp = 0x27
	OpRegionOp    AMLOp = 0x80
	OpFile        AMLOp = 0x81
	OpDevice      AMLOp = 0x82
	OpPowerSource AMLOp = 0x84

	OpLocal   AMLOp = 0x60
	OpArg     AMLOp = 0x68
	OpStore   AMLOp = 0x70
	OpDerefof AMLOp = 0x83
	OpNotify  AMLOp = 0x86
	OpSizeOf  AMLOp = 0x87

	OpObjectType AMLOp = 0x8E
	OpLNot       AMLOp = 0x92
	OpLEqual     AMLOp = 0x93
	OpLGreater   AMLOp = 0x94
	OpLLess      AMLOp = 0x95
	OpToBuffer   AMLOp = 0x96
	OpToInteger  AMLOp = 0x99

	OpMid    AMLOp = 0x9E
	OpIf     AMLOp = 0xA0
	OpElse   AMLOp = 0xA1
	OpWhile  AMLOp = 0xA2
	OpReturn AMLOp = 0xA4
	OpOnes   AMLOp = 0xFF

	IOPortDesc            AMLOp = 0x47
	EndTag                AMLOp = 0x79
	Mem32FixedDesc        AMLOp = 0x86
	DWordAddressSpaceDesc AMLOp = 0x87
	WordAddressSpaceDesc  AMLOp = 0x88
	ExtIRQDesc            AMLOp = 0x89
	QWordAddressSpaceDesc AMLOp = 0x8A
)

type BinaryAMLOp uint8

const (
	OpAdd        BinaryAMLOp = 0x72
	OpConcat     BinaryAMLOp = 0x73
	OpSubstract  BinaryAMLOp = 0x74
	OpMultiply   BinaryAMLOp = 0x77
	OpShiftLeft  BinaryAMLOp = 0x79
	OpShiftRight BinaryAMLOp = 0x7A
	OpAND        BinaryAMLOp = 0x7B
	OpNAND       BinaryAMLOp = 0x7C
	OpOR         BinaryAMLOp = 0x7D
	OpNOR        BinaryAMLOp = 0x7E
	OpXOR        BinaryAMLOp = 0x7F

	OpConcatRes    BinaryAMLOp = 0x84
	OpMod          BinaryAMLOp = 0x85
	OpIndex        BinaryAMLOp = 0x88
	OpCreateDWFile BinaryAMLOp = 0x8A
	OpCreateQWFile BinaryAMLOp = 0x8F
	OpToString     BinaryAMLOp = 0x9C
)

type AML struct {
	buf bytes.Buffer
}

func NewAML() *AML {
	return &AML{
		buf: bytes.Buffer{},
	}
}

func (a *AML) ToBytes() []byte {
	return a.buf.Bytes()
}

func (a *AML) Zero() *AML {
	a.buf.WriteByte(byte(OpZero))

	return a
}

func (a *AML) One() *AML {
	a.buf.WriteByte(byte(OpOne))

	return a
}

func (a *AML) Ones() *AML {
	a.buf.WriteByte(byte(OpOnes))

	return a
}

func (a *AML) Path(str string) *AML {
	if strings.HasPrefix(str, "\\") {
		a.buf.WriteByte('\\')

		str = strings.Trim(str, "\\")
	}

	strs := strings.Split(str, ".")

	for _, substring := range strs {
		if len(substring) > 4 {
			return nil
		}

		a.buf.WriteString(substring)
	}

	return a
}

func (a *AML) Bytes(b byte) *AML {
	a.buf.WriteByte(byte(OpBytePrefix))
	a.buf.WriteByte(b)

	return a
}

func (a *AML) Word(w uint16) *AML {
	a.buf.WriteByte(byte(OpWordPrefix))

	data := make([]byte, 2)

	binary.LittleEndian.PutUint16(data, w)

	a.buf.Write(data)

	return a
}

func (a *AML) DWord(dw uint32) *AML {
	a.buf.WriteByte(byte(OpDWordPrefix))

	data := make([]byte, 4)

	binary.LittleEndian.PutUint32(data, dw)

	a.buf.Write(data)

	return a
}

func (a *AML) QWord(qw uint64) *AML {
	a.buf.WriteByte(byte(OpQWordPrefix))

	data := make([]byte, 8)

	binary.LittleEndian.PutUint64(data, qw)

	a.buf.Write(data)

	return a
}

func (a *AML) Name(path string, inner *AML) *AML {
	a.buf.WriteByte(byte(OpName))
	a.Path(path)
	a.buf.Write(inner.ToBytes())

	return a
}

func (a *AML) EISAName(str string) *AML {
	if len(str) != 7 {
		return nil
	}

	var eisaid uint32
	eisaid |= (uint32(str[0]) - 'A' + 1&0x1F) << 10
	eisaid |= (uint32(str[1]) - 'A' + 1&0x1F) << 5
	eisaid |= (uint32(str[2]) - 'A' + 1&0x1F)

	n1, err := strconv.ParseUint(str[3:], 16, 32)
	if err != nil {
		return nil
	}

	eisaid |= (uint32(n1) << 16)

	data := make([]byte, 4)

	binary.LittleEndian.PutUint32(data, eisaid)

	a.buf.Write(data)

	return a
}

func (a *AML) String(str string) *AML {
	a.buf.WriteByte(byte(OpString))

	for _, substr := range str {
		a.buf.WriteByte(byte(substr))
	}

	a.buf.WriteByte(0x0)

	return a
}

const (
	pkgLen1 = 63
	pkgLen2 = 4096
	pkgLen3 = 1048573
)

func CalcPkgLength(length uint32, includepkg bool) []byte {
	var lenlen uint32

	if length < pkgLen1 { // nolint:gocritic
		lenlen = 1
	} else if length < pkgLen2 {
		lenlen = 2
	} else if length < pkgLen3 {
		lenlen = 3
	} else {
		lenlen = 4
	}

	ret := make([]byte, lenlen)

	if includepkg {
		length += lenlen
	}

	switch lenlen {
	case 1:
		ret[0] = uint8(length)
	case 2:
		ret[0] = (uint8(1) << 6) | uint8(length&0xf)
		ret[1] = uint8(length >> 4)
	case 3:
		ret[0] = (uint8(2) << 6) | uint8(length&0xf)
		ret[1] = uint8(length >> 4)
		ret[2] = uint8(length >> 12)
	case 4:
		ret[0] = (uint8(3) << 6) | uint8(length&0xf)
		ret[1] = uint8(length >> 4)
		ret[2] = uint8(length >> 12)
		ret[3] = uint8(length >> 20)
	}

	return ret
}

func (a *AML) ResourceTemplate(inner *AML) *AML {
	var buf1, buf2 bytes.Buffer

	buf1.Write(inner.ToBytes())
	buf1.WriteByte(byte(EndTag))
	buf1.WriteByte(0x0)

	dlenb := make([]byte, 4)
	dlen := uint32(buf1.Len() + len(dlenb))

	binary.LittleEndian.PutUint32(dlenb, dlen)

	pkglen := CalcPkgLength(dlen+4, true)

	buf2.Write(pkglen)
	buf2.Write(dlenb)
	buf2.Write(buf1.Bytes())
	a.buf.WriteByte(byte(OpBuffer))
	a.buf.Write(buf2.Bytes())

	return a
}

func (a *AML) Memory32Fixed(base, length uint32, rw bool) *AML {
	readwrite := uint8(0)

	a.buf.WriteByte(byte(Mem32FixedDesc))
	a.buf.WriteByte(0x09)
	a.buf.WriteByte(0x0)

	if rw {
		readwrite = 1
	}

	a.buf.WriteByte(readwrite)
	a.DWord(base)
	a.DWord(length)

	return a
}

func (a *AML) IO(min, max uint16, align, length uint8) *AML {
	a.buf.WriteByte(byte(IOPortDesc))
	a.buf.WriteByte(0x1)
	a.Word(min)
	a.Word(max)
	a.buf.WriteByte(align)
	a.buf.WriteByte(length)

	return a
}

func (a *AML) Interrupt(consumer, edgetrig, activelow, shared bool, number uint32) *AML {
	flags := uint8(0)

	if consumer {
		flags = 0x1
	}

	if edgetrig {
		flags |= 1 << 1
	}

	if activelow {
		flags |= 1 << 2
	}

	if shared {
		flags |= 1 << 3
	}

	a.buf.WriteByte(byte(ExtIRQDesc))
	a.Word(0x6)
	a.buf.WriteByte(flags)
	a.buf.WriteByte(1)
	a.DWord(number)

	return a
}

func (a *AML) Device(path string, children *AML) *AML {
	aml := NewAML()
	aml.Path(path)

	aml.buf.Write(children.ToBytes())

	amllen := uint32(aml.buf.Len())

	pkglen := CalcPkgLength(amllen, true)

	a.buf.WriteByte(byte(OpExtPrefix))
	a.buf.WriteByte(byte(OpDevice))
	a.buf.Write(pkglen)
	a.buf.Write(aml.ToBytes())

	return a
}

func (a *AML) Method(path string, args uint8, serialize bool, children *AML) *AML {
	amlbuf := NewAML()

	amlbuf.Path(path)

	flags := args & 0x7

	if serialize {
		flags |= 1 << 3
	}

	amlbuf.buf.WriteByte(flags)
	amlbuf.buf.Write(children.ToBytes())

	datlen := uint32(amlbuf.buf.Len())

	pkglen := CalcPkgLength(datlen, true)

	a.buf.WriteByte(byte(OpMethod))
	a.buf.Write(pkglen)
	a.buf.Write(amlbuf.ToBytes())

	return a
}

const (
	FieldAccessTypeAny uint8 = 0 + iota
	FieldAccessTypeByte
	FieldAccessTypeWord
	FieldAccessTypeDWord
	FieldAccessTypeQWord
	FieldAccessTypeBuffer

	FieldUpdateRulePreserve     uint8 = 0
	FieldUpdateRuleWriteAsOnes  uint8 = 1
	FieldUpdateRuleWriteAsZeros uint8 = 2

	FieldEntryTypeNamed    uint8 = 0
	FieldEntryTypeReserved uint8 = 1
)

type FieldEntry interface {
	Name() string
	Length() uint32
}

type FieldEntryNamed struct {
	name   string
	length uint32
}

func NewFieldEntryNamed(name string, l uint32) FieldEntryNamed {
	return FieldEntryNamed{name: name, length: l}
}

func (f FieldEntryNamed) Name() string {
	return f.name
}

func (f FieldEntryNamed) Length() uint32 {
	return f.length
}

type FieldEntryReserved struct {
	length uint32
}

func NewFieldEntryReserved(l uint32) FieldEntryReserved {
	return FieldEntryReserved{length: l}
}

func (f FieldEntryReserved) Name() string {
	return "reserved" // Give it a undesirable name just in case.
}

func (f FieldEntryReserved) Length() uint32 {
	return f.length
}

func (a *AML) Field(path string, accessType uint8, lockrule bool, updaterule uint8, entries ...FieldEntry) *AML {
	amlbuf := NewAML()

	amlbuf.Path(path)

	flags := accessType | updaterule<<5

	if lockrule {
		flags |= 1 << 4
	}

	amlbuf.buf.WriteByte(flags)

	for _, entry := range entries {
		switch e := entry.(type) {
		case *FieldEntryNamed:
			amlbuf.buf.Write([]byte(e.Name()))
			amlbuf.buf.Write(CalcPkgLength(e.Length(), false))
		case *FieldEntryReserved:
			amlbuf.buf.WriteByte(0x0)
			amlbuf.buf.Write(CalcPkgLength(e.Length(), false))
		}
	}

	pkglen := CalcPkgLength(uint32(amlbuf.buf.Len()), true)

	a.buf.WriteByte(byte(OpExtPrefix))
	a.buf.WriteByte(byte(OpFile))
	a.buf.Write(pkglen)
	a.buf.Write(amlbuf.ToBytes())

	return a
}

const (
	OpRegionSpaceSysMem uint8 = 0 + iota
	OpRegionSpaceSysIO
	OpRegionSpacePCIConf
	OpRegionSpaceEmbControl
	OpRegionSpaceSMBus
	OpRegionSpaceSysCMOS
	OpRegionSpacePCIBarTarget
	OpRegionSpaceIPMI
	OpRegionSpaceGPIO
	OpRegionSpaceGenSerialBus
)

func (a *AML) OpRegion(path string, space uint8, offset *AML, length *AML) *AML {
	a.buf.WriteByte(byte(OpExtPrefix))
	a.buf.WriteByte(byte(OpRegionOp))
	a.Path(path)
	a.buf.WriteByte(space)
	a.buf.Write(offset.ToBytes())
	a.buf.Write(length.ToBytes())

	return a
}

func (a *AML) Store(name *AML, value *AML) *AML {
	a.buf.WriteByte(byte(OpStore))
	a.buf.Write(name.ToBytes())
	a.buf.Write(value.ToBytes())

	return a
}

func (a *AML) Mutex(path string, syncLevel uint8) *AML {
	a.buf.WriteByte(byte(OpExtPrefix))
	a.buf.WriteByte(byte(OpMutex))
	a.Path(path)
	a.buf.WriteByte(syncLevel)

	return a
}

func (a *AML) Acquire(path string, timeout uint16) *AML {
	a.buf.WriteByte(byte(OpExtPrefix))
	a.buf.WriteByte(byte(OpAcquire))
	a.Path(path)
	a.Word(timeout)

	return a
}

func (a *AML) Release(path string) *AML {
	a.buf.WriteByte(byte(OpExtPrefix))
	a.buf.WriteByte(byte(OpRelease))
	a.Path(path)

	return a
}

func (a *AML) MethodCall(path string, args *AML) *AML {
	a.Path(path)
	a.buf.Write(args.ToBytes())

	return a
}

func (a *AML) Return(op AML) *AML {
	a.buf.WriteByte(byte(OpReturn))
	a.buf.Write(op.ToBytes())

	return a
}

func (a *AML) BinaryOp(op BinaryAMLOp, operandA *AML, operandB *AML, target *AML) *AML {
	a.buf.WriteByte(byte(op))
	a.buf.Write(operandA.ToBytes())
	a.buf.Write(operandB.ToBytes())
	a.buf.Write(target.ToBytes())

	return a
}

const (
	TypeAddressSpaceMemory    uint8 = 0
	TypeAddressSpaceIO        uint8 = 1
	TypeAddressSpaceBusnumber uint8 = 2

	TFlagNotCachable    uint8 = 0
	TFlagReadWrite      uint8 = 1
	TFlagCachable       uint8 = 2
	TFlagWriteCombining uint8 = 3
	TFlagPrefetchable   uint8 = 4
)

func (a *AML) AddressSpace64(addrtype uint8, min, max uint64, tflags uint8, translation []byte) *AML {
	a.buf.WriteByte(byte(QWordAddressSpaceDesc))

	length := 43

	blen := make([]byte, 2)

	binary.LittleEndian.PutUint16(blen, uint16(length))
	a.buf.Write(blen)

	a.buf.WriteByte(addrtype)

	genflags := uint8(1<<2 | 1<<3)

	a.buf.WriteByte(genflags)

	a.buf.WriteByte(tflags)

	a.QWord(0x0)
	a.QWord(min)
	a.QWord(max)
	a.QWord(0x0)
	a.QWord(max - min + 1)

	return a
}

func (a *AML) BufferTerm() *AML { return a }

func (a *AML) BufferData() *AML { return a }

func (a *AML) Package() *AML { return a }

func (a *AML) If() *AML { return a }

func (a *AML) Else() *AML { return a }

func (a *AML) Arg(arg uint8) *AML {
	a.buf.WriteByte(uint8(OpArg) + arg)

	return a
}

func (a *AML) Local() *AML { return a }

func (a *AML) Scope() *AML { return a }

func (a *AML) Notify() *AML { return a }

func (a *AML) While() *AML { return a }

func (a *AML) CreateField() *AML { return a }

func (a *AML) Mid() *AML { return a }
