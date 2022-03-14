package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/term"
)

func main() {
	kernelPath, initrdPath, params, tapIfName, nCpus, err := flag.ParseArgs(os.Args)
	if err != nil {
		panic(err)
	}

	m, err := machine.New(nCpus, tapIfName)
	if err != nil {
		panic(err)
	}

	if err := m.LoadLinux(kernelPath, initrdPath, params); err != nil {
		panic(err)
	}

	for i := 0; i < nCpus; i++ {
		go func(cpuId int) {
			if err = m.RunInfiniteLoop(cpuId); err != nil {
				panic(err)
			}
		}(i)
	}

	if !term.IsTerminal() {
		fmt.Fprintln(os.Stderr, "this is not terminal and does not accept input")
		select {}
	}

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
