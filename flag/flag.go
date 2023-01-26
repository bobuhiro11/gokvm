package flag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses a size string as number[gGmMkK]. The multiplier is optional,
// and if not set, the unit passed in is used. The number can be any base and
// size.
func ParseSize(s, unit string) (int, error) {
	sz := strings.TrimRight(s, "gGmMkK")
	if len(sz) == 0 {
		return -1, fmt.Errorf("%q:can't parse as num[gGmMkK]:%w", s, strconv.ErrSyntax)
	}

	amt, err := strconv.ParseUint(sz, 0, 0)
	if err != nil {
		return -1, err
	}

	if len(s) > len(sz) {
		unit = s[len(sz):]
	}

	switch unit {
	case "G", "g":
		return int(amt) << 30, nil
	case "M", "m":
		return int(amt) << 20, nil
	case "K", "k":
		return int(amt) << 10, nil
	case "":
		return int(amt), nil
	}

	return -1, fmt.Errorf("can not parse %q as num[gGmMkK]:%w", s, strconv.ErrSyntax)
}

// ParseArgs calls flag.Parse and returns strings for
// device, kernel, initrd, bootparams, tapIfName, disk, memSize, and nCpus.
// another coding anti-pattern from golangci-lint.
func ParseArgs(args []string) (kvmPath, kernel, initrd, params,
	tapIfName, disk string, nCpus, memSize, traceCount int,
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

	flag.IntVar(&nCpus, "c", 1, "number of cpus")

	msize := flag.String("m", "1G", "memory size: as number[gGmM], optional units, defaults to G")
	tc := flag.String("T", "0", "how many instructions to skip between trace prints -- 0 means tracing disabled")

	flag.Parse()

	if err = flag.CommandLine.Parse(args[1:]); err != nil {
		return
	}

	if memSize, err = ParseSize(*msize, "g"); err != nil {
		return
	}

	if traceCount, err = ParseSize(*tc, ""); err != nil {
		return
	}

	// personally, I think the naked return is easier, it avoids
	// getting things in the wrong order. But, in fact, there are
	// too many returns to this function, I think it needs
	// a config struct.
	// And, weirdly, it accepts the naked return on all previous return
	// statements. Go figure.
	return kvmPath, kernel, initrd, params,
		tapIfName, disk, nCpus, memSize, traceCount, nil
}
