package bootproto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/ioutil"
)

const (
	BootProtoMagicSignature = 0x53726448
)

// https://www.kernel.org/doc/html/latest/x86/boot.html
type BootProto struct {
	SetupSects          uint8
	RootFlags           uint16
	SysSize             uint32
	RAMSize             uint16
	VidMode             uint16
	RootDev             uint16
	BootFlag            uint16
	Jump                uint16
	Header              uint32
	Version             uint16
	ReadModeSwitch      uint32
	StartSysSeg         uint16
	KernelVersion       uint16
	TypeOfLoader        uint8
	LoadFlags           uint8
	SetupMoveSize       uint16
	Code32Start         uint32
	RamdiskImage        uint32
	RamdiskSize         uint32
	BootsectKludge      uint32
	HeapEndPtr          uint16
	ExtLoaderVer        uint8
	ExtLoaderType       uint8
	CmdlinePtr          uint32
	InitrdAddrMax       uint32
	KernelAlignment     uint32
	RelocatableKernel   uint8
	MinAlignment        uint8
	XloadFlags          uint16
	CmdlineSize         uint32
	HardwareSubarch     uint32
	HardwareSubarchData uint64
	PayloadOffset       uint32
	PayloadLength       uint32
	SetupData           uint64
	PrefAddress         uint64
	InitSize            uint32
	HandoverOffset      uint32
	KernelInfoOffset    uint32
}

var ErrorSignatureNotMatch = errors.New("signature not match in bzImage")

func New(bzImagePath string) (*BootProto, error) {
	b := &BootProto{}

	bzImage, err := ioutil.ReadFile(bzImagePath)
	if err != nil {
		return b, err
	}

	reader := bytes.NewReader(bzImage[0x01F1:])
	err = binary.Read(reader, binary.LittleEndian, b)

	if err != nil {
		return b, err
	}

	if b.Header != BootProtoMagicSignature {
		return b, ErrorSignatureNotMatch
	}

	return b, nil
}

// NOTE: base address for boot protocol is 0x01F1 in guest physical memory.
func (b *BootProto) Bytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, b); err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}
