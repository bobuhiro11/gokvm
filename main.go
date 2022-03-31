package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/term"
)

func main() {
	kernelPath, initrdPath, params, tapIfName, nCpus, err := flag.ParseArgs(os.Args)
	if err != nil {
		log.Fatalf("ParseArgs: %v", err)
	}

	m, err := machine.New(nCpus, tapIfName)
	if err != nil {
		log.Fatalf("%v", err)
	}

	if err := m.LoadLinux(kernelPath, initrdPath, params); err != nil {
		log.Fatalf("%v", err)
	}

	for i := 0; i < nCpus; i++ {
		go func(cpuId int) {
			if err = m.RunInfiniteLoop(cpuId); err != nil {
				log.Fatalf("%v", err)
			}
		}(i)
	}

	if !term.IsTerminal() {
		fmt.Fprintln(os.Stderr, "this is not terminal and does not accept input")
		select {}
	}

	restoreMode, err := term.SetRawMode()
	if err != nil {
		log.Fatalf("%v", err)
	}

	defer restoreMode()

	var before byte = 0

	in := bufio.NewReader(os.Stdin)

	for {
		b, err := in.ReadByte()
		if err != nil {
			log.Printf("%v", err)

			break
		}
		m.GetInputChan() <- b

		if len(m.GetInputChan()) > 0 {
			if err := m.InjectSerialIRQ(); err != nil {
				log.Printf("InjectSerialIRQ: %v", err)
			}
		}

		if before == 0x1 && b == 'x' {
			break
		}

		before = b
	}
}
