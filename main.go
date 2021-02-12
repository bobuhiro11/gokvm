package main

import (
	"bufio"
	"os"

	"github.com/nmi/gokvm/machine"
	"github.com/nmi/gokvm/term"
)

func main() {
	m, err := machine.New()
	if err != nil {
		panic(err)
	}

	if err := m.LoadLinux("./bzImage", "./initrd"); err != nil {
		panic(err)
	}

	go func() {
		if err = m.RunInfiniteLoop(); err != nil {
			panic(err)
		}
	}()

	restoreMode, err := term.SetRawMode()
	if err != nil {
		panic(err)
	}

	defer restoreMode()

	var before byte = 0

	in := bufio.NewReader(os.Stdin)

	for {
		b, err := in.ReadByte()
		if err != nil {
			panic(err)
		}
		m.GetInputChan() <- b

		if len(m.GetInputChan()) > 0 {
			m.InjectSerialIRQ()
		}

		if before == 0x1 && b == 'x' {
			break
		}

		before = b
	}
}
