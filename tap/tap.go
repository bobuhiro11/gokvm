package tap

import (
	"syscall"
	"unsafe"
)

type Tap struct {
	Fd int
	Name string
}

type ifReq struct {
	Name  [0x10]byte
	Flags uint16
	pad   [0x28 - 0x10 - 2]byte
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
	t := &Tap{}
	var err error

	if t.Fd, err = syscall.Open("/dev/net/tun", syscall.O_RDWR, 0); err != nil {
		return t, err
	}

	ifr := ifReq{
		Flags: syscall.IFF_TAP|syscall.IFF_NO_PI,
	}
	copy(ifr.Name[:0xf], name)
	if _, err = ioctl(uintptr(t.Fd), syscall.TUNSETIFF, uintptr(unsafe.Pointer(&ifr))); err != nil {
		return t, err
	}

	return t, nil
}

func (t Tap) Tx (bytes []byte) error {
	syscall.Write(t.Fd, bytes)
	return nil
}

