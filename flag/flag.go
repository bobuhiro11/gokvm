package flag

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
)

var ErrorInvalidSubcommands = errors.New("expected 'boot' or 'probe' subcommands")

type BootArgs struct {
	Kernel     string
	MemSize    int
	NCPUs      int
	Dev        string
	Initrd     string
	Params     string
	TapIfName  string
	Disk       string
	TraceCount int
}

func parseBootArgs(args []string) (*BootArgs, error) {
	bootCmd := flag.NewFlagSet("boot subcommand", flag.ExitOnError)
	c := &BootArgs{}

	bootCmd.StringVar(&c.Dev, "D", "/dev/kvm", "path of kvm device")
	bootCmd.StringVar(&c.Kernel, "k", "./bzImage", "kernel image path")
	bootCmd.StringVar(&c.Initrd, "i", "", "initrd path")
	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	bootCmd.StringVar(&c.Params, "p", `console=ttyS0 earlyprintk=serial `+
		`noapic noacpi notsc nowatchdog `+
		`nmi_watchdog=0 debug apic=debug show_lapic=all mitigations=off `+
		`lapic tsc_early_khz=2000 `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" `+
		`pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init `+
		`gokvm.ipv4_addr=192.168.20.1/24`,
		"kernel command-line parameters")
	bootCmd.StringVar(&c.TapIfName, "t", "", `name of tap interface. `+
		`If the string is an empty, no tap intarface is created. (default"")`)
	bootCmd.StringVar(&c.Disk, "d", "", "path of disk file (for /dev/vda)")

	bootCmd.IntVar(&c.NCPUs, "c", 1, "number of cpus")

	msize := bootCmd.String("m", "1G",
		"memory size: as number[gGmM], optional units, defaults to G")
	tc := bootCmd.String("T", "0",
		"how many instructions to skip between trace prints -- 0 means tracing disabled")

	var err error

	if err = bootCmd.Parse(args); err != nil {
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

type ProbeArgs struct{}

func parseProbeArgs(args []string) (*ProbeArgs, error) {
	probeCmd := flag.NewFlagSet("probe subcommand", flag.ExitOnError)
	c := &ProbeArgs{}

	if err := probeCmd.Parse(args); err != nil {
		return nil, err
	}

	return c, nil
}

func ParseArgs(args []string) (*BootArgs, *ProbeArgs, error) {
	if len(args) < 2 {
		return nil, nil, ErrorInvalidSubcommands
	}

	switch args[1] {
	case "boot":
		conf, err := parseBootArgs(args[2:])

		return conf, nil, err

	case "probe":
		conf, err := parseProbeArgs(args[2:])

		return nil, conf, err
	}

	return nil, nil, ErrorInvalidSubcommands
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
