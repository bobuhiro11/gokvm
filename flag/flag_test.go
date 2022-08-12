package flag_test

import (
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
	}

	kvmPath, kernel, initrd, params, tapIfName, disk, nCpus, err := flag.ParseArgs(args)
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
		t.Error("invalid path of disk file")
	}

	if nCpus != 2 {
		t.Error("invalid number of vcpus")
	}
}
