package vmm_test

// TestDiskMigration boots a VM with a marked disk image, live-migrates it to
// a second VMM instance, and verifies the destination disk received the
// marker.  This mirrors the manual demo from the development session.
//
// Requirements: must run as root inside an unshare --user --net namespace
// (satisfied by `make test` and the CI matrix).

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/bobuhiro11/gokvm/vmm"
)

const (
	migKernel = "../bzImage"
	migInitrd = "../initrd"
	migVDA    = "../vda.img"

	migSrcTap     = "tap-mig-src"
	migSrcGuestIP = "192.168.50.1"
	migSrcHostIP  = "192.168.50.2"
	migPrefixLen  = "24"
	migListenAddr = "127.0.0.1:7780"
	migMarkerOff  = 512 * 1024 // byte offset in disk image for the test marker
)

var migMarker = []byte("DISK_MIGRATION_CI_MARKER") //nolint:gochecknoglobals

func TestDiskMigration(t *testing.T) { //nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skip("TestDiskMigration requires root (run inside unshare --user --net --map-root-user)")
	}

	// Loopback is DOWN in a fresh user+net namespace; dst listens on 127.0.0.1.
	if err := exec.Command("ip", "link", "set", "lo", "up").Run(); err != nil {
		t.Fatalf("ip link set lo up: %v", err)
	}

	// ── Prepare disk images ──────────────────────────────────────────────────
	srcDisk := copyDiskForMigTest(t, migVDA, "src-mig-")
	dstDisk := copyDiskForMigTest(t, migVDA, "dst-mig-")

	// Write the marker into the src disk at a fixed offset.
	srcF, err := os.OpenFile(srcDisk, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open src disk: %v", err)
	}

	if _, err := srcF.WriteAt(migMarker, migMarkerOff); err != nil {
		srcF.Close()
		t.Fatalf("write marker to src disk: %v", err)
	}

	srcF.Close()

	// Confirm dst does NOT yet contain the marker.
	dstBefore, _ := os.ReadFile(dstDisk)
	if bytes.Contains(dstBefore, migMarker) {
		t.Fatal("marker unexpectedly present in dst disk before migration")
	}

	t.Logf("BEFORE migration: marker written to src at offset %d; not in dst ✓", migMarkerOff)

	// ── Start dst VMM in Incoming mode ───────────────────────────────────────
	dst := vmm.New(vmm.Config{
		Dev:     "/dev/kvm",
		Disk:    dstDisk,
		NCPUs:   1,
		MemSize: 512 << 20,
	})

	dstErrC := make(chan error, 1)

	go func() { dstErrC <- dst.Incoming(migListenAddr) }()

	// Give dst a moment to start listening before src dials.
	time.Sleep(200 * time.Millisecond)

	// ── Boot src VMM ─────────────────────────────────────────────────────────
	src := vmm.New(vmm.Config{
		Dev:    "/dev/kvm",
		Kernel: migKernel,
		Initrd: migInitrd,
		Params: fmt.Sprintf(`console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
			`lapic tsc_early_khz=2000 pci=realloc=off virtio_pci.force_legacy=1 `+
			`rdinit=/init init=/init gokvm.ipv4_addr=%s/%s`,
			migSrcGuestIP, migPrefixLen),
		TapIfName: migSrcTap,
		Disk:      srcDisk,
		NCPUs:     1,
		MemSize:   512 << 20,
	})

	if err := src.Init(); err != nil {
		t.Fatalf("src.Init: %v", err)
	}

	if err := src.Setup(); err != nil {
		t.Fatalf("src.Setup: %v", err)
	}

	// Discard serial output (suppress noise; logged only on failure).
	src.GetSerial().SetOutput(io.Discard)
	src.GetInputChan()

	if err := src.InjectSerialIRQ(); err != nil {
		t.Logf("InjectSerialIRQ: %v (non-fatal)", err)
	}

	src.RunData()

	t.Cleanup(func() {
		// MigrateTo already calls Machine.Close on success; calling it again
		// is safe (atomic store + harmless SIGURG delivery).
		if src.Machine != nil {
			src.Machine.Close()
		}

		if dst.Machine != nil {
			dst.Machine.Close()
		}
	})

	// Deadline watchdog: dump diagnostics and close the VM if the test
	// deadline approaches before migration completes.
	if dl, ok := t.Deadline(); ok {
		go func() {
			margin := 60 * time.Second
			timer := time.NewTimer(time.Until(dl) - margin)

			defer timer.Stop()

			<-timer.C
			t.Errorf("watchdog: deadline %s approaching; closing src VM", dl.Format(time.RFC3339))

			if src.Machine != nil {
				src.Machine.Close()
			}
		}()
	}

	go func() {
		if err := src.Machine.RunInfiniteLoop(0); err != nil {
			t.Logf("src RunInfiniteLoop: %v", err)
		}
	}()

	// Bring up src tap networking.
	for _, args := range [][]string{
		{"ip", "link", "set", migSrcTap, "up"},
		{"ip", "addr", "add", migSrcHostIP + "/" + migPrefixLen, "dev", migSrcTap},
	} {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil { //nolint:gosec
			t.Fatalf("network setup %v: %v", args, err)
		}
	}

	// ── Wait for src VM to boot (ping) ───────────────────────────────────────
	t.Logf("waiting for src VM to boot (guest %s) …", migSrcGuestIP)
	migWaitForPing(t, migSrcGuestIP)
	t.Logf("src VM booted at %s", time.Now().Format(time.RFC3339))

	// ── Trigger live migration ───────────────────────────────────────────────
	t.Logf("starting live migration to %s …", migListenAddr)

	if err := src.MigrateTo(migListenAddr); err != nil {
		t.Fatalf("MigrateTo: %v", err)
	}

	t.Logf("MigrateTo returned (src VM stopped) at %s", time.Now().Format(time.RFC3339))

	// ── Verify disk contents ─────────────────────────────────────────────────
	// By the time MigrateTo returns, dst has received MsgDiskFull and written
	// the dst disk (causality: disk write → MsgDone rx → MsgReady tx → MsgReady
	// rx on src → MigrateTo returns).

	dstAfter, err := os.ReadFile(dstDisk)
	if err != nil {
		t.Fatalf("read dst disk after migration: %v", err)
	}

	srcAfter, err := os.ReadFile(srcDisk)
	if err != nil {
		t.Fatalf("read src disk after migration: %v", err)
	}

	// Check marker transferred.
	dstSlice := dstAfter[migMarkerOff : migMarkerOff+len(migMarker)]

	if !bytes.Equal(dstSlice, migMarker) {
		t.Errorf("FAIL: marker not found in dst disk after migration\ngot  %q\nwant %q",
			dstSlice, migMarker)
	} else {
		t.Logf("AFTER migration: marker found in dst disk ✓")
	}

	// Check full disk equality.
	if bytes.Equal(srcAfter, dstAfter) {
		t.Logf("AFTER migration: src disk == dst disk ✓")
	} else {
		t.Errorf("FAIL: src and dst disk images differ after migration")
	}

	// Stop the dst VM so the test terminates cleanly.
	if dst.Machine != nil {
		dst.Machine.Close()
	}

	select {
	case err := <-dstErrC:
		t.Logf("dst Incoming goroutine returned: %v", err)
	case <-time.After(10 * time.Second):
		t.Log("dst still running after 10s (OK – VM is live)")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func copyDiskForMigTest(t *testing.T, src, prefix string) string {
	t.Helper()

	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("copyDiskForMigTest open %s: %v", src, err)
	}

	defer in.Close()

	out, err := os.CreateTemp("", prefix+"*.img")
	if err != nil {
		t.Fatalf("copyDiskForMigTest create temp: %v", err)
	}

	t.Cleanup(func() { os.Remove(out.Name()) })

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatalf("copyDiskForMigTest copy: %v", err)
	}

	out.Close()

	return out.Name()
}

func migWaitForPing(t *testing.T, ip string) {
	t.Helper()

	deadline := time.Now().Add(300 * time.Second)

	for {
		out, err := exec.Command("ping", ip, "-c", "1", "-W", "1").CombinedOutput()
		if err == nil {
			t.Logf("ping %s ok", ip)

			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("ping %s timed out after 300s: %s", ip, out)
		}

		time.Sleep(2 * time.Second)
	}
}
