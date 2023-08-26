package main

import (
	"log"

	"github.com/bobuhiro11/gokvm/flag"
)

func main() {
	if err := flag.Parse().Run(); err != nil {
		log.Fatal(err)
	}
}
