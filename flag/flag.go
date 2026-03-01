package flag

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
)

var ErrorInvalidSubcommands = errors.New("expected 'boot', 'probe', 'incoming', or 'migrate' subcommands")

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

// IncomingArgs holds arguments for the 'incoming' subcommand (migration destination).
type IncomingArgs struct {
	Listen    string // TCP address to listen on, e.g. ":4444"
	Dev       string
	MemSize   int
	NCPUs     int
	TapIfName string
	Disk      string
}

// MigrateArgs holds arguments for the 'migrate' subcommand.
type MigrateArgs struct {
	Sock string // path to the source VM control socket
	To   string // destination address, e.g. "host:4444"
}

func parseIncomingArgs(args []string) (*IncomingArgs, error) {
	cmd := flag.NewFlagSet("incoming subcommand", flag.ExitOnError)
	c := &IncomingArgs{}

	cmd.StringVar(&c.Listen, "l", ":4444", "TCP address to listen for incoming migration")
	cmd.StringVar(&c.Dev, "D", "/dev/kvm", "path of kvm device")
	cmd.StringVar(&c.TapIfName, "t", "", "name of tap interface")
	cmd.StringVar(&c.Disk, "d", "", "path of disk file (for /dev/vda)")
	cmd.IntVar(&c.NCPUs, "c", 1, "number of cpus (must match source)")

	msize := cmd.String("m", "1G", "memory size (must match source)")

	if err := cmd.Parse(args); err != nil {
		return nil, err
	}

	var err error

	if c.MemSize, err = ParseSize(*msize, "g"); err != nil {
		return nil, err
	}

	return c, nil
}

func parseMigrateArgs(args []string) (*MigrateArgs, error) {
	cmd := flag.NewFlagSet("migrate subcommand", flag.ExitOnError)
	c := &MigrateArgs{}

	cmd.StringVar(&c.Sock, "s", "", "path to the source VM control socket (e.g. /tmp/gokvm-<pid>.sock)")
	cmd.StringVar(&c.To, "to", "", "destination address (host:port)")

	if err := cmd.Parse(args); err != nil {
		return nil, err
	}

	if c.Sock == "" || c.To == "" {
		return nil, errors.New("migrate: -s <sock> and -to <addr> are required")
	}

	return c, nil
}

func parseProbeArgs(args []string) (*ProbeArgs, error) {
	probeCmd := flag.NewFlagSet("probe subcommand", flag.ExitOnError)
	c := &ProbeArgs{}

	if err := probeCmd.Parse(args); err != nil {
		return nil, err
	}

	return c, nil
}

func ParseArgs(args []string) (*BootArgs, *ProbeArgs, *IncomingArgs, *MigrateArgs, error) {
	if len(args) < 2 {
		return nil, nil, nil, nil, ErrorInvalidSubcommands
	}

	switch args[1] {
	case "boot":
		conf, err := parseBootArgs(args[2:])

		return conf, nil, nil, nil, err

	case "probe":
		conf, err := parseProbeArgs(args[2:])

		return nil, conf, nil, nil, err

	case "incoming":
		conf, err := parseIncomingArgs(args[2:])

		return nil, nil, conf, nil, err

	case "migrate":
		conf, err := parseMigrateArgs(args[2:])

		return nil, nil, nil, conf, err
	}

	return nil, nil, nil, nil, ErrorInvalidSubcommands
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
