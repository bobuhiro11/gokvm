//go:build !test

package main

import (
	"log"

	"github.com/bobuhiro11/gokvm/flag"
)

func main() {
	if err := flag.Parse(); err != nil {
		log.Fatal(err)
	}
}
