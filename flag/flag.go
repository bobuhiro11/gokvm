package flag

import (
	"flag"
)

func ParseArgs(args []string) (string, string, string, int, error) {
	kernel := flag.String("k", "./bzImage", "kernel image path")
	initrd := flag.String("i", "./initrd", "initrd path")
	nCpus := flag.Int("c", 1, "number of cpus")

	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	params := flag.String("p", `console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
		`debug apic=debug show_lapic=all mitigations=off lapic `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf" pci=realloc=off`, "kernel command-line parameters")

	flag.Parse()

	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return "", "", "", 0, err
	}

	return *kernel, *initrd, *params, *nCpus, nil
}
