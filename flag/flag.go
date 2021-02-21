package flag

import (
	"flag"
)

func ParseArgs(args []string) (string, string, string, int, error) {
	kernel := flag.String("k", "./bzImage", "kernel image path")
	initrd := flag.String("i", "./initrd", "initrd path")
	params := flag.String("p", "console=ttyS0", "kernel command-line parameters")
	nCpus := flag.Int("c", 1, "number of cpus")

	flag.Parse()

	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return "", "", "", 0, err
	}

	return *kernel, *initrd, *params, *nCpus, nil
}
