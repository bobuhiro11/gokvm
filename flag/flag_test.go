package flag_test

import (
	"testing"

	"github.com/nmi/gokvm/flag"
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
	}

	kernel, initrd, params, err := flag.ParseArgs(args)
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
}
