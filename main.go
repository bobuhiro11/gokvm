package main

import (
	"log"
	"os"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/probe"
	"github.com/bobuhiro11/gokvm/vmm"
)

func main() {
	bootArgs, probeArgs, err := flag.ParseArgs(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	if bootArgs != nil {
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

		vmm := vmm.New(*c)

		if err := vmm.Init(); err != nil {
			log.Fatal(err)
		}

		if err := vmm.Setup(); err != nil {
			log.Fatal(err)
		}

		if err := vmm.Boot(); err != nil {
			log.Fatal(err)
		}
	}

	if probeArgs != nil {
		if err := probe.KVMCapabilities(); err != nil {
			log.Fatal(err)
		}
	}
}
