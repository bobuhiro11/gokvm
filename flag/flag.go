package flag

import (
	"fmt"
	"strconv"
	"strings"
)

type context struct{}

type cli struct {
	Start StartCmd `cmd:"" help:"Starts a new VM"`
	Debug DebugCmd `cmd:"" help:"Prints KVM capabilities"`
}

type StartCmd struct {
	Kernel     string `flag:"" short:"k" name:"kernel" default:"./bzImage" help:"Path to linux kernel" type:"path"`
	MemSize    string `flag:"" short:"m" name:"memsize" default:"1G" help:"memory size: as number[gGmM]"`
	NCPUs      int    `flag:"" short:"c" name:"ncpus" default:"1" help:"Number of cpus"`
	Dev        string `flag:"" short:"D" name:"kvmDevice" default:"/dev/kvm" help:"Path to Linux KVM device" type:"path"`
	Initrd     string `flag:"" short:"i" name:"initrd" default:"./initrd" help:"Path to initrd" type:"path"`
	Params     string `flag:"" short:"p" name:"params" help:"Linux kernel cmdline parameters"`
	TapIfName  string `flag:"" short:"t" name:"tapifname" help:"name of tap interface" type:"path"`
	Disk       string `flag:"" short:"d" name:"disk" help:"path of disk file" type:"path"`
	TraceCount string `flag:"" short:"T" name:"traceCount" default:"0" help:"instructions to skip between trace prints"`
}

type DebugCmd struct{}

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
