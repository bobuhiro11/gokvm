package term

import (
	"syscall"
	"unsafe"
)

type termios struct {
	Iflag  uint32
	Oflag  uint32
	Cflag  uint32
	Lflag  uint32
	Line   uint8
	Cc     [19]uint8
	Ispeed uint32
	Ospeed uint32
}

func read(fd int) (termios, error) {
	var t termios

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, uintptr(fd), 0x5401,
		uintptr(unsafe.Pointer(&t)))

	var err error = nil
	if errno != 0 {
		err = errno
	}

	return t, err
}

func write(fd int, t termios) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, uintptr(fd), 0x5402,
		uintptr(unsafe.Pointer(&t)))

	var err error = nil
	if errno != 0 {
		err = errno
	}

	return err
}

func SetRawMode() (func(), error) {
	t, err := read(0)
	if err != nil {
		return func() {}, err
	}

	oldTermios := t

	t.Iflag &^= 0b10111101011
	t.Oflag &^= 1
	t.Lflag &^= 0b1000000001001011
	t.Cflag &^= 0b01001000 | 0b100000000
	t.Cflag |= 48
	t.Cc[6] = 1
	t.Cc[5] = 0

	return func() {
		_ = write(0, oldTermios)
	}, write(0, t)
}
