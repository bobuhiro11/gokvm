package main

import (
	"log"
	"os"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/probe"
	"github.com/bobuhiro11/gokvm/vmm"
)

func main() {
	// This line break is required by golangci-lint but
	// such breaks are considered an anti-pattern
	// at Google.
	c, err := flag.ParseArgs(os.Args)
	if err != nil {
		log.Fatalf("ParseArgs: %v", err)
	}

	if c.Debug {
		if err := probe.KVMCapabilities(); err != nil {
			log.Fatalf("kvm capabilities: %v", err)
		}
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
