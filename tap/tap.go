package tap

import (
	"fmt"
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

func fcntl(fd, op, arg uintptr) (uintptr, error) {
	res, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL, fd, op, arg)

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

	// issue SIGIO if tap interface is accessed.
	if _, err = fcntl(uintptr(t.fd), syscall.F_SETSIG, 0); err != nil {
		fmt.Printf("syscall.F_SETSIG failed\r\n")
		return t, err
	}

	// const DN_ACCESS = 0x00000001
	// const DN_MODIFY = 0x00000002
	// if _, err = fcntl(uintptr(t.fd), syscall.F_NOTIFY, DN_MODIFY); err != nil {
	// 	fmt.Printf("syscall.F_NOTIFY failed\r\n")
	// 	return t, err
	// }

	// enable non-blocing IO for tap interface
	var flags uintptr
	if flags, err = fcntl(uintptr(t.fd), syscall.F_GETFL, 0); err != nil {
		fmt.Printf("syscall.F_GETFL failed\r\n")
		return t, err
	}

	if _, err = fcntl(uintptr(t.fd), syscall.F_SETFL, flags | syscall.O_NONBLOCK | syscall.O_ASYNC); err != nil {
		fmt.Printf("syscall.F_SETFL failed\r\n")
		return t, err
	}

	return t, nil
}

func (t *Tap) Close() error {
	return syscall.Close(t.fd)
}

func (t Tap) Write(buf []byte) (n int, err error) {
	fmt.Printf("tap.Write called\r\n")
	return syscall.Write(t.fd, buf)
}

func (t Tap) Read(buf []byte) (n int, err error) {
	fmt.Printf("tap.Read called\r\n")
	return syscall.Read(t.fd, buf)
}
