package flag_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/bobuhiro11/gokvm/flag"
)

func TestParsesize(t *testing.T) { // nolint:paralleltest
	for _, tt := range []struct {
		name string
		unit string
		m    string
		amt  int
		err  error
	}{
		{name: "badsuffix", m: "1T", amt: -1, err: strconv.ErrSyntax},
		{name: "1G", m: "1G", amt: 1 << 30, err: nil},
		{name: "1g", m: "1g", amt: 1 << 30, err: nil},
		{name: "1M", m: "1M", amt: 1 << 20, err: nil},
		{name: "1m", m: "1m", amt: 1 << 20, err: nil},
		{name: "1K", m: "1K", amt: 1 << 10, err: nil},
		{name: "1k", m: "1k", amt: 1 << 10, err: nil},
		{name: "1 with unit k", m: "1", unit: "k", amt: 1 << 10, err: nil},
		{name: "1 with unit \"\"", m: "1", unit: "", amt: 1, err: nil},
		{name: "8192m", m: "8192m", amt: 8192 << 20, err: nil},
		{name: "bogusgarbage", m: "123411;3413234134", amt: -1, err: strconv.ErrSyntax},
		{name: "bogusgarbagemsuffix", m: "123411;3413234134m", amt: -1, err: strconv.ErrSyntax},
		{name: "bogustoobig", m: "0xfffffffffffffffffffffff", amt: -1, err: strconv.ErrRange},
	} {
		amt, err := flag.ParseSize(tt.m, tt.unit)
		if !errors.Is(err, tt.err) || amt != tt.amt {
			t.Errorf("%s:parseMemSize(%s): got (%d, %v), want (%d, %v)", tt.name, tt.m, amt, err, tt.amt, tt.err)
		}
	}
}

func TestParseBootArgs(t *testing.T) {
	t.Parallel()

	args := []string{
		"gokvm",
		"boot",
		"-i",
		"initrd_path",
		"-k",
		"kernel_path",
		"-p",
		"params",
		"-t",
		"tap_if_name",
		"-c",
		"2",
		"-d",
		"disk_path",
		"-m",
		"1G",
		"-T",
		"1M",
	}

	c, _, _, _, err := flag.ParseArgs(args) //nolint:dogsled
	if err != nil {
		t.Fatal(err)
	}

	if c.Dev != "/dev/kvm" {
		t.Error("invalid kvm  path")
	}

	if c.Kernel != "kernel_path" {
		t.Error("invalid kernel image path")
	}

	if c.Initrd != "initrd_path" {
		t.Error("invalid initrd path")
	}

	if c.Params != "params" {
		t.Error("invalid kernel command-line parameters")
	}

	if c.TapIfName != "tap_if_name" {
		t.Error("invalid name of tap interface")
	}

	if c.Disk != "disk_path" {
		t.Errorf("invalid path of disk file: got %v, want %v", c.Disk, "disk_path")
	}

	if c.NCPUs != 2 {
		t.Error("invalid number of vcpus")
	}

	if c.MemSize != 1<<30 {
		t.Errorf("msize: got %#x, want %#x", c.MemSize, 1<<30)
	}

	if c.TraceCount != 1<<20 {
		t.Errorf("trace: got %#x, want %#x", c.TraceCount, 1<<20)
	}
}

func TestParseBootArgsWithDefaults(t *testing.T) {
	t.Parallel()

	args := []string{
		"gokvm",
		"boot",
	}

	c, _, _, _, err := flag.ParseArgs(args) //nolint:dogsled
	if err != nil {
		t.Fatal(err)
	}

	if c.Initrd != "" {
		t.Error("invalid initrd path")
	}

	if c.Params != `console=ttyS0 earlyprintk=serial `+
		`noapic noacpi notsc nowatchdog `+
		`nmi_watchdog=0 debug apic=debug show_lapic=all mitigations=off `+
		`lapic tsc_early_khz=2000 `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" `+
		`pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init `+
		`gokvm.ipv4_addr=192.168.20.1/24` {
		t.Error("invalid kernel command-line parameters")
	}

	if c.TapIfName != "" {
		t.Error("invalid name of tap interface")
	}

	if c.Disk != "" {
		t.Errorf("invalid path of disk file: got %v, want %v", c.Disk, "disk_path")
	}

	if c.NCPUs != 1 {
		t.Error("invalid number of vcpus")
	}

	if c.MemSize != 1<<30 {
		t.Errorf("msize: got %#x, want %#x", c.MemSize, 1<<30)
	}

	if c.TraceCount != 0 {
		t.Errorf("trace: got %#x, want %#x", c.TraceCount, 1<<20)
	}
}

func TestParseProbeArgs(t *testing.T) {
	t.Parallel()

	args := []string{
		"gokvm",
		"probe",
	}

	_, probeConfig, _, _, err := flag.ParseArgs(args) //nolint:dogsled
	if err != nil {
		t.Fatal(err)
	}

	if probeConfig == nil {
		t.Fatal("probeConfig is nil")
	}
}
