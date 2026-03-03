package vmm_test

// migrate_stats_test.go – unit tests for vmm migration helpers exposed via
// export_test.go.  Tests that do not require KVM run in parallel; tests that
// do require KVM are guarded by an os.Getuid() check.

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/vmm"
)

// ── MigrateStats.String ──────────────────────────────────────────────────────

func TestMigrateStatsStringMemoryOnly(t *testing.T) {
	t.Parallel()

	s := vmm.MigrateStats{
		MemBytes: 512 << 20,
		MemPages: 131072,
	}

	got := s.String()

	if !strings.HasPrefix(got, "memory:") {
		t.Errorf("String() does not start with 'memory:': %q", got)
	}

	if strings.Contains(got, "disk:") {
		t.Errorf("String() should not contain 'disk:' when DiskBytes==0: %q", got)
	}

	want := fmt.Sprintf("memory: %d bytes (%d pages, %d MiB)", s.MemBytes, s.MemPages, s.MemBytes>>20)
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestMigrateStatsStringWithDisk(t *testing.T) {
	t.Parallel()

	s := vmm.MigrateStats{
		MemBytes:   512 << 20,
		MemPages:   131072,
		DiskBytes:  1 << 20,
		DiskBlocks: 2048,
	}

	got := s.String()

	if !strings.Contains(got, "memory:") {
		t.Errorf("String() missing memory line: %q", got)
	}

	if !strings.Contains(got, "disk:") {
		t.Errorf("String() missing disk line: %q", got)
	}

	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), got)
	}

	wantDisk := fmt.Sprintf("disk:   %d bytes (%d blocks, %d MiB)", s.DiskBytes, s.DiskBlocks, s.DiskBytes>>20)
	if lines[1] != wantDisk {
		t.Errorf("disk line = %q, want %q", lines[1], wantDisk)
	}
}

// ── controlSocketPath ────────────────────────────────────────────────────────

func TestControlSocketPathFormat(t *testing.T) {
	t.Parallel()

	want := "/tmp/gokvm-12345.sock"
	got := vmm.ControlSocketPath(12345)

	if got != want {
		t.Errorf("ControlSocketPath(12345) = %q, want %q", got, want)
	}
}

// ── StartControlSocket / handleControl ──────────────────────────────────────

// TestControlSocketUnknownCommand verifies that an unrecognised command
// returns "ERROR unknown command" without requiring a KVM machine.
func TestControlSocketUnknownCommand(t *testing.T) { //nolint:paralleltest
	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	sockPath, err := v.StartControlSocket()
	if err != nil {
		t.Fatalf("StartControlSocket: %v", err)
	}

	defer os.Remove(sockPath)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial control socket: %v", err)
	}

	defer conn.Close()

	if _, err := fmt.Fprint(conn, "NOTACOMMAND\n"); err != nil {
		t.Fatalf("write command: %v", err)
	}

	if err := conn.(*net.UnixConn).CloseWrite(); err != nil { //nolint:forcetypeassert
		t.Fatalf("CloseWrite: %v", err)
	}

	resp := make([]byte, 256)

	n, _ := conn.Read(resp)
	response := string(resp[:n])

	if !strings.Contains(response, "ERROR unknown command") {
		t.Errorf("expected 'ERROR unknown command', got %q", response)
	}
}

// TestControlSocketMIGRATEDialFail verifies that a MIGRATE command targeting
// an unreachable address returns an ERROR response.
func TestControlSocketMIGRATEDialFail(t *testing.T) { //nolint:paralleltest
	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	sockPath, err := v.StartControlSocket()
	if err != nil {
		t.Fatalf("StartControlSocket: %v", err)
	}

	defer os.Remove(sockPath)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial control socket: %v", err)
	}

	defer conn.Close()

	// Port 1 is almost always refused immediately on Linux.
	if _, err := fmt.Fprint(conn, "MIGRATE 127.0.0.1:1\n"); err != nil {
		t.Fatalf("write command: %v", err)
	}

	if err := conn.(*net.UnixConn).CloseWrite(); err != nil { //nolint:forcetypeassert
		t.Fatalf("CloseWrite: %v", err)
	}

	var resp []byte

	buf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	for {
		n, readErr := conn.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
		}

		if readErr != nil {
			break
		}
	}

	response := string(resp)

	if !strings.Contains(response, "ERROR") {
		t.Errorf("expected ERROR response, got %q", response)
	}
}

// ── applyDirtyPages ──────────────────────────────────────────────────────────

// TestApplyDirtyPagesBadBitmapLen verifies that a bitmap length not a
// multiple of 8 returns an error without touching the machine.
func TestApplyDirtyPagesBadBitmapLen(t *testing.T) {
	t.Parallel()

	// 7 bytes – not a multiple of 8.
	err := vmm.ApplyDirtyPages(nil, make([]byte, 7), nil)
	if err == nil {
		t.Fatal("expected error for odd bitmap length, got nil")
	}
}

// TestApplyDirtyPagesTruncated verifies that a bitmap marking more pages than
// the pageData contains returns an error.
func TestApplyDirtyPagesTruncated(t *testing.T) { //nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skip("requires root (run inside unshare --user --net --map-root-user)")
	}

	m, err := machine.New("/dev/kvm", 1, machine.MinMemSize)
	if err != nil {
		t.Fatalf("machine.New: %v", err)
	}

	defer m.Close()

	// Mark page 0 as dirty but supply only 1 byte of page data (< 4096).
	bitmapBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bitmapBytes, 1) // bit 0 set → page 0 dirty

	pageData := []byte{0xAB} // only 1 byte – should cause truncated error

	if err := vmm.ApplyDirtyPages(m, bitmapBytes, pageData); err == nil {
		t.Fatal("expected truncated error, got nil")
	}
}

// TestApplyDirtyPagesSuccess verifies that apply correctly writes pages into
// the machine's guest memory when bitmap and page data are consistent.
func TestApplyDirtyPagesSuccess(t *testing.T) { //nolint:paralleltest
	if os.Getuid() != 0 {
		t.Skip("requires root (run inside unshare --user --net --map-root-user)")
	}

	m, err := machine.New("/dev/kvm", 1, machine.MinMemSize)
	if err != nil {
		t.Fatalf("machine.New: %v", err)
	}

	defer m.Close()

	const pageSize = 4096

	// Mark pages 0 and 2 as dirty (bitmap = 0b0101 = 5).
	bitmapBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bitmapBytes, 5)

	page0 := bytes32mib(pageSize, 0xAA)
	page2 := bytes32mib(pageSize, 0xCC)

	pageData := make([]byte, 0, 2*pageSize)
	pageData = append(pageData, page0...)
	pageData = append(pageData, page2...)

	if err := vmm.ApplyDirtyPages(m, bitmapBytes, pageData); err != nil {
		t.Fatalf("applyDirtyPages: %v", err)
	}

	mem := m.Mem()

	for i := 0; i < pageSize; i++ {
		if mem[i] != 0xAA {
			t.Fatalf("page0[%d] = %#x, want 0xAA", i, mem[i])
		}
	}

	for i := 2 * pageSize; i < 3*pageSize; i++ {
		if mem[i] != 0xCC {
			t.Fatalf("page2[%d] = %#x, want 0xCC", i, mem[i])
		}
	}
}

// bytes32mib returns a slice of n bytes all set to v.
func bytes32mib(n int, v byte) []byte {
	b := make([]byte, n)

	for i := range b {
		b[i] = v
	}

	return b
}
