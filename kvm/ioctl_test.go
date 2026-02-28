package kvm_test

import (
	"os"
	"testing"

	"github.com/bobuhiro11/gokvm/kvm"
)

func TestIoctlEINTRRetry(t *testing.T) {
	t.Parallel()

	devKVM, err := os.OpenFile(
		"/dev/kvm", os.O_RDWR, 0o644,
	)
	if err != nil {
		t.Fatal(err)
	}

	defer devKVM.Close()

	// KVM_GET_API_VERSION exercises the Ioctl retry loop.
	// It must succeed despite the EINTR-retry wrapper.
	_, err = kvm.GetAPIVersion(devKVM.Fd())
	if err != nil {
		t.Fatalf("GetAPIVersion failed: %v", err)
	}
}
