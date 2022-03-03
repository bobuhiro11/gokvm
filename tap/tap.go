package tap

import (
	"syscall"
	"unsafe"
)

const ifNameSize = 0x10

type Tap struct {
	fd int
}

type ifReq struct {
	Name  [ifNameSize]byte
	Flags uint16
	_     [0x28 - ifNameSize - 2]byte
}

func ioctl(fd, op, arg uintptr) (uintptr, error) {
	res, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, fd, op, arg)

	var err error = nil
	if errno != 0 {
		err = errno
	}

	return res, err
}

func New(name string) (*Tap, error) {
	var err error

	t := &Tap{}

	if t.fd, err = syscall.Open("/dev/net/tun", syscall.O_RDWR, 0); err != nil {
		return t, err
	}

	ifr := ifReq{
		Name:  [ifNameSize]byte{},
		Flags: syscall.IFF_TAP | syscall.IFF_NO_PI,
	}
	copy(ifr.Name[:ifNameSize-1], name)

	ifrPtr := uintptr(unsafe.Pointer(&ifr))
	if _, err = ioctl(uintptr(t.fd), syscall.TUNSETIFF, ifrPtr); err != nil {
		return t, err
	}

	return t, nil
}

func (t *Tap) Close() error {
	return syscall.Close(t.fd)
}

func (t Tap) Tx(bytes []byte) error {
	if _, err := syscall.Write(t.fd, bytes); err != nil {
		return err
	}

	return nil
}
