package tap_test

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"

	"github.com/bobuhiro11/gokvm/tap"
)

func requireCAP(t *testing.T, err error) {
	t.Helper()

	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skip("requires CAP_NET_ADMIN: ", err)
	}
}

func TestNew(t *testing.T) { // nolint:paralleltest
	tap, err := tap.New("test_tap")
	if err != nil {
		requireCAP(t, err)
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
		requireCAP(t, err)
		t.Fatal(err)
	}

	if err := exec.Command("ip", "link", "set", "test_write", "up").Run(); err != nil {
		t.Fatal(err)
	}

	if _, err := tap.Write(make([]byte, 20)); err != nil {
		t.Fatal(err)
	}

	if err := tap.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRead(t *testing.T) { // nolint:paralleltest
	tap, err := tap.New("test_read")
	if err != nil {
		requireCAP(t, err)
		t.Fatal(err)
	}

	if err := exec.Command("ip", "link", "set", "test_read", "up").Run(); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 20)
	if _, err := tap.Read(buf); err != nil &&
		!errors.Is(err, syscall.EAGAIN) {
		t.Fatal(err)
	}

	if err := tap.Close(); err != nil {
		t.Fatal(err)
	}
}
