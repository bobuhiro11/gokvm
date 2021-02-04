package main

import (
	"github.com/nmi/gokvm/kvm"
)

func main() {
	g, err := kvm.NewLinuxGuest("./bzImage", "./initrd")
	if err != nil {
		panic(err)
	}

	ioportHandler := func(port uint32, isIn bool, value byte) {
	}
	_ = g.Run(ioportHandler)
}
