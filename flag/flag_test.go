package flag_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/bobuhiro11/gokvm/flag"
)

func TestParseArg(t *testing.T) {
	t.Parallel()

	args := []string{
		"gokvm",
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

	kvmPath, kernel, initrd, params, tapIfName, disk, nCpus, msize, tc, err := flag.ParseArgs(args)
	if err != nil {
		t.Fatal(err)
	}

	if kvmPath != "/dev/kvm" {
		t.Error("invalid kvm  path")
	}

	if kernel != "kernel_path" {
		t.Error("invalid kernel image path")
	}

	if initrd != "initrd_path" {
		t.Error("invalid initrd path")
	}

	if params != "params" {
		t.Error("invalid kernel command-line parameters")
	}

	if tapIfName != "tap_if_name" {
		t.Error("invalid name of tap interface")
	}

	if disk != "disk_path" {
		t.Errorf("invalid path of disk file: got %v, want %v", disk, "disk_path")
	}

	if nCpus != 2 {
		t.Error("invalid number of vcpus")
	}

	if msize != 1<<30 {
		t.Errorf("msize: got %#x, want %#x", msize, 1<<30)
	}

	if tc != 1<<20 {
		t.Errorf("trace: got %#x, want %#x", tc, 1<<20)
	}
}

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
