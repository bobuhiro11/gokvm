package flag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

func ParseMemSize(s string) (int, error) {
	sz := strings.TrimRight(s, "gGmMkK")
	if len(sz) == 0 {
		return -1, fmt.Errorf("%q:can't parse as num[gGmMkK]:%w", s, strconv.ErrSyntax)
	}

	amt, err := strconv.ParseUint(sz, 0, 0)
	if err != nil {
		return -1, err
	}

	if len(sz) == len(s) {
		return int(amt) << 30, nil
	}

	switch s[len(sz):] {
	case "G", "g":
		return int(amt) << 30, nil
	case "M", "m":
		return int(amt) << 20, nil
	case "K", "k":
		return int(amt) << 10, nil
	}

	return -1, fmt.Errorf("can not parse %q as num[gGmMkK]:%w", s, strconv.ErrSyntax)
}

// ParseArgs calls flag.Parse and returns strings for
// device, kernel, initrd, bootparams, tapIfName, disk, memSize, and nCpus.
// another coding anti-pattern from golangci-lint.
func ParseArgs(args []string) (kvmPath, kernel, initrd, params,
	tapIfName, disk string, nCpus, memSize int,
	trace bool,
	err error,
) {
	flag.StringVar(&kvmPath, "D", "/dev/kvm", "path of kvm device")
	flag.StringVar(&kernel, "k", "./bzImage", "kernel image path")
	flag.StringVar(&initrd, "i", "./initrd", "initrd path")
	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	flag.StringVar(&params, "p", `console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
		`debug apic=debug show_lapic=all mitigations=off lapic tsc_early_khz=2000 `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init`, "kernel command-line parameters")
	flag.StringVar(&tapIfName, "t", "tap", "name of tap interface")
	flag.StringVar(&disk, "d", "/dev/zero", "path of disk file (for /dev/vda)")

	flag.BoolVar(&trace, "T", false, "single-step process and print each step")

	flag.IntVar(&nCpus, "c", 1, "number of cpus")

	msize := flag.String("m", "1G", "memory size: as number[gGmM], optional units, defaults to G")

	flag.Parse()

	if err = flag.CommandLine.Parse(args[1:]); err != nil {
		return
	}

	if memSize, err = ParseMemSize(*msize); err != nil {
		return
	}

	// personally, I think the naked return is easier, it avoids
	// getting things in the wrong order. But, in fact, there are
	// too many returns to this function, I think it needs
	// a config struct.
	return kvmPath, kernel, initrd, params,
		tapIfName, disk, nCpus, memSize, trace, nil
}
