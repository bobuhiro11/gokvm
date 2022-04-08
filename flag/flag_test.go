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

	kernel, initrd, params, tapIfName, disk, nCpus, err := flag.ParseArgs(args)
	if err != nil {
		t.Fatal(err)
	}

	if kernel != "kernel_path" {
		t.Fatal("invalid kernel image path")
	}

	if initrd != "initrd_path" {
		t.Fatal("invalid initrd path")
	}

	if params != "params" {
		t.Fatal("invalid kernel command-line parameters")
	}

	if tapIfName != "tap_if_name" {
		t.Fatal("invalid name of tap interface")
	}

	if disk != "disk_path" {
		t.Fatal("invalid path of disk file")
	}

	if nCpus != 2 {
		t.Fatal("invalid number of vcpus")
	}
}
