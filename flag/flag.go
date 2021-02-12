package flag

import (
	"flag"
)

func ParseArgs(args []string) (string, string, error) {
	kernel := flag.String("k", "./bzImage", "kernel image path")
	initrd := flag.String("i", "./initrd", "initrd path")

	flag.Parse()

	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return "", "", err
	}

	return *kernel, *initrd, nil
}
