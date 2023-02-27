package flag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// Config defines the configuration of the
// virtual machine, as determined by flags.
type Config struct {
	Debug      bool
	Dev        string
	Kernel     string
	Initrd     string
	Params     string
	TapIfName  string
	Disk       string
	NCPUs      int
	MemSize    int
	TraceCount int
}

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

// ParseArgs calls flag.Parse and a *Config or error.
func ParseArgs(args []string) (*Config, error) {
	c := &Config{}
	flag.BoolVar(&c.Debug, "debug", false, "This flag provides a list of KVM capabilities and nothing more")
	flag.StringVar(&c.Dev, "D", "/dev/kvm", "path of kvm device")
	flag.StringVar(&c.Kernel, "k", "./bzImage", "kernel image path")
	flag.StringVar(&c.Initrd, "i", "./initrd", "initrd path")
	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	flag.StringVar(&c.Params, "p", `console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
		`debug apic=debug show_lapic=all mitigations=off lapic tsc_early_khz=2000 `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init `+
		`gokvm.ipv4_addr=192.168.20.1/24`, "kernel command-line parameters")
	flag.StringVar(&c.TapIfName, "t", "", `name of tap interface. `+
		`If the string is an empty, no tap intarface is created. (default "")`)
	flag.StringVar(&c.Disk, "d", "/dev/zero", "path of disk file (for /dev/vda)")

	flag.IntVar(&c.NCPUs, "c", 1, "number of cpus")

	msize := flag.String("m", "1G", "memory size: as number[gGmM], optional units, defaults to G")
	tc := flag.String("T", "0", "how many instructions to skip between trace prints -- 0 means tracing disabled")

	flag.Parse()

	var err error
	if err = flag.CommandLine.Parse(args[1:]); err != nil {
		return nil, err
	}

	if c.MemSize, err = ParseSize(*msize, "g"); err != nil {
		return nil, err
	}

	if c.TraceCount, err = ParseSize(*tc, ""); err != nil {
		return nil, err
	}

	return c, nil
}
