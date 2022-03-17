package tap_test

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"

	"github.com/bobuhiro11/gokvm/tap"
)

func TestNew(t *testing.T) { // nolint:paralleltest
	tap, err := tap.New("test_tap")
	if err != nil {
		t.Fatal(err)
	}

	err = tap.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestWrite(t *testing.T) { // nolint:paralleltest
	tap, err := tap.New("test_write")
	if err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("ip", "link", "set", "test_write", "up").Run(); err != nil {
		t.Fatal(err)
	}

	if _, err := tap.Write(make([]byte, 20)); err != nil {
		t.Fatal(err)
	}

	_ = tap.Close()
}

func TestRead(t *testing.T) { // nolint:paralleltest
	tap, err := tap.New("test_read")
	if err != nil {
		t.Fatal(err)
	}

	if err := exec.Command("ip", "link", "set", "test_read", "up").Run(); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 20)
	if _, err := tap.Read(buf); !errors.Is(err, syscall.EAGAIN) {
		t.Fatal(err)
	}

	_ = tap.Close()
}
