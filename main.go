package main

import (
	"github.com/nmi/gokvm/kvm"
)

func main() {
	g, err := kvm.NewLinuxGuest("./bzImage", "./initrd")
	if err != nil {
		panic(err)
	}

	if err = g.Run(); err != nil {
		panic(err)
	}
}
