package flag_test

import (
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/alecthomas/kong"
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

func TestCmdlineBootParsing(t *testing.T) {
	t.Parallel()

	args := os.Args
	defer func() {
		os.Args = args
	}()

	os.Args = []string{
		"gokvm",
		"boot",
		"-D",
		"/dev/kvm",
		"-k",
		"kernel_path",
		"-i",
		"initrd_path",
		"-m",
		"1G",
		"-c",
		"2",
		"-t",
		"tap0",
		"-d",
		"/dev/null",
		"-T",
		"1",
	}

	kong.Parse(&flag.CLI{}, kong.Exit(func(_ int) { t.Fatal("parsing failed") }))
}

func TestCmdlineProbeParsing(t *testing.T) {
	t.Parallel()

	args := os.Args
	defer func() {
		os.Args = args
	}()

	os.Args = []string{
		"gokvm",
		"probe",
	}

	kong.Parse(&flag.CLI{}, kong.Exit(func(_ int) { t.Fatal("parsing failed") }))
}
