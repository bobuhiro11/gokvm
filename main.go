package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/probe"
	"github.com/bobuhiro11/gokvm/vmm"
)

func main() {
	bootArgs, probeArgs, incomingArgs, migrateArgs, err := flag.ParseArgs(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	if bootArgs != nil {
		bootVM(bootArgs)
	}

	if probeArgs != nil {
		if err := probe.KVMCapabilities(); err != nil {
			log.Fatal(err)
		}

		if err := probe.CPUID(); err != nil {
			log.Fatal(err)
		}
	}

	if incomingArgs != nil {
		c := vmm.Config{
			Dev:       incomingArgs.Dev,
			NCPUs:     incomingArgs.NCPUs,
			MemSize:   incomingArgs.MemSize,
			TapIfName: incomingArgs.TapIfName,
			Disk:      incomingArgs.Disk,
		}

		v := vmm.New(c)

		if err := v.Incoming(incomingArgs.Listen); err != nil {
			log.Fatal(err)
		}
	}

	if migrateArgs != nil {
		conn, err := net.Dial("unix", migrateArgs.Sock)
		if err != nil {
			log.Fatalf("connect to control socket %s: %v", migrateArgs.Sock, err)
		}

		cmd := fmt.Sprintf("MIGRATE %s\n", migrateArgs.To)
		if _, err := fmt.Fprint(conn, cmd); err != nil {
			conn.Close()
			log.Fatal(err)
		}

		resp := make([]byte, 256)

		n, _ := conn.Read(resp)

		fmt.Printf("%s", resp[:n])

		conn.Close()
	}
}

func bootVM(bootArgs *flag.BootArgs) {
	c := &vmm.Config{
		Dev:        bootArgs.Dev,
		Kernel:     bootArgs.Kernel,
		Initrd:     bootArgs.Initrd,
		Params:     bootArgs.Params,
		TapIfName:  bootArgs.TapIfName,
		Disk:       bootArgs.Disk,
		NCPUs:      bootArgs.NCPUs,
		MemSize:    bootArgs.MemSize,
		TraceCount: bootArgs.TraceCount,
	}

	v := vmm.New(*c)

	if err := v.Init(); err != nil {
		log.Fatal(err)
	}

	if err := v.Setup(); err != nil {
		log.Fatal(err)
	}

	sockPath, err := v.StartControlSocket()
	if err != nil {
		log.Printf("warning: control socket unavailable: %v", err)
	} else {
		fmt.Printf("control socket: %s\r\n", sockPath)
	}

	if err := v.Boot(); err != nil {
		log.Fatal(err)
	}
}
