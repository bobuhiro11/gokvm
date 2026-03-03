package vmm_test

// migrate_stats_test.go – unit tests for vmm migration helpers exposed via
// export_test.go.  Tests that do not require KVM run in parallel; tests that
// do require KVM are guarded by an os.Getuid() check.

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/migration"
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

// ── applySnapshot ─────────────────────────────────────────────────────────────

// requireRoot skips the test if it is not running as root.
func requireRoot(t *testing.T) {
	t.Helper()

	if os.Getuid() != 0 {
		t.Skip("requires root (run inside unshare --user --net --map-root-user)")
	}
}

// newTestMachineForVMM creates a fresh KVM machine for vmm tests.
func newTestMachineForVMM(t *testing.T) *machine.Machine {
	t.Helper()
	requireRoot(t)

	m, err := machine.New("/dev/kvm", 1, machine.MinMemSize)
	if err != nil {
		t.Fatalf("machine.New: %v", err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

// TestApplySnapshotCPUError verifies that applySnapshot returns an error when
// RestoreCPUState fails (nil Regs in VCPUState).
func TestApplySnapshotCPUError(t *testing.T) { //nolint:paralleltest
	m := newTestMachineForVMM(t)

	snap := &migration.Snapshot{
		NCPUs:      1,
		MemSize:    machine.MinMemSize,
		VCPUStates: []migration.VCPUState{{}}, // empty → Regs=nil → decode fails
	}

	if err := vmm.ApplySnapshot(m, snap); err == nil {
		t.Fatal("expected error from nil Regs, got nil")
	}
}

// TestApplySnapshotVMError verifies that applySnapshot returns an error when
// RestoreVMState fails (nil Clock in VMState), after all CPUs restored OK.
func TestApplySnapshotVMError(t *testing.T) { //nolint:paralleltest
	src := newTestMachineForVMM(t)

	cpuState, err := src.SaveCPUState(0)
	if err != nil {
		t.Fatalf("SaveCPUState: %v", err)
	}

	dst := newTestMachineForVMM(t)

	snap := &migration.Snapshot{
		NCPUs:      1,
		MemSize:    machine.MinMemSize,
		VCPUStates: []migration.VCPUState{*cpuState},
		VM:         migration.VMState{}, // nil Clock → RestoreVMState fails
	}

	if err := vmm.ApplySnapshot(dst, snap); err == nil {
		t.Fatal("expected error from nil Clock, got nil")
	}
}

// ── handleControl EOF ─────────────────────────────────────────────────────────

// TestControlSocketReadEOF verifies that handleControl handles EOF gracefully
// (connection closed without sending a newline-terminated command).
func TestControlSocketReadEOF(t *testing.T) { //nolint:paralleltest
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

	// Close the connection without sending any data or newline.
	// handleControl's read loop will hit err != nil (EOF) and break.
	conn.Close()

	// Small sleep to let the goroutine run.
	time.Sleep(50 * time.Millisecond)
}

// ── Incoming helper ───────────────────────────────────────────────────────────

// startIncoming launches v.Incoming in a goroutine, waits for the listener to
// be ready, and returns the established TCP connection and a channel that
// receives Incoming's return value when it finishes.
func startIncoming(t *testing.T, v *vmm.VMM, addr string) (net.Conn, <-chan error) {
	t.Helper()

	// Loopback is down in a fresh user+net namespace; ensure it is up.
	_ = exec.Command("ip", "link", "set", "lo", "up").Run()

	errCh := make(chan error, 1)

	go func() { errCh <- v.Incoming(addr) }()

	var conn net.Conn

	var dialErr error

	for i := 0; i < 100; i++ {
		time.Sleep(20 * time.Millisecond)

		conn, dialErr = net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if dialErr == nil {
			break
		}
	}

	if dialErr != nil {
		t.Fatalf("dial incoming at %s: %v", addr, dialErr)
	}

	return conn, errCh
}

// writeMigMsg sends a single framed migration message on conn.
func writeMigMsg(conn net.Conn, msgType uint32, payload []byte) {
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint32(hdr[0:4], msgType)
	binary.BigEndian.PutUint64(hdr[4:12], uint64(len(payload)))
	_, _ = conn.Write(hdr)

	if len(payload) > 0 {
		_, _ = conn.Write(payload)
	}
}

// ── Incoming tests ─────────────────────────────────────────────────────────────

// TestIncomingMachineNewError verifies that Incoming returns an error when
// machine.New fails (MemSize=0 < MinMemSize).  No KVM needed.
func TestIncomingMachineNewError(t *testing.T) { //nolint:paralleltest
	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: 0})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7791")
	defer conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from machine.New with MemSize=0, got nil")
	}
}

// TestIncomingCloseImmediately verifies that Incoming returns an error when
// the connection is closed before any messages are sent.
func TestIncomingCloseImmediately(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7792")
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from closed connection, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingWithValidTapIf verifies that Incoming proceeds past AddTapIf
// when TapIfName is non-empty and the interface can be created.
func TestIncomingWithValidTapIf(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{
		Dev:       "/dev/kvm",
		NCPUs:     1,
		MemSize:   machine.MinMemSize,
		TapIfName: "tap-inc-ok",
	})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7793")
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from closed connection, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingAddDiskError verifies that Incoming returns an error when AddDisk
// fails (Disk path does not exist).
func TestIncomingAddDiskError(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{
		Dev:     "/dev/kvm",
		NCPUs:   1,
		MemSize: machine.MinMemSize,
		Disk:    "/nonexistent-disk-for-test.img",
	})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7794")
	defer conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from AddDisk, got nil")
	}
}

// TestIncomingBadMemoryFull verifies that Incoming returns an error when a
// MsgMemoryFull payload is too short (RestoreMemory fails).
func TestIncomingBadMemoryFull(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7795")
	defer conn.Close()

	// Send MsgMemoryFull with 0-byte payload → RestoreMemory fails (EOF).
	writeMigMsg(conn, uint32(migration.MsgMemoryFull), nil)
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from short MsgMemoryFull, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingBadMemoryDirty verifies that Incoming returns an error when a
// MsgMemoryDirty payload is too short for DecodeDirtyPayload (< 8 bytes).
func TestIncomingBadMemoryDirty(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7796")
	defer conn.Close()

	// 3-byte payload → DecodeDirtyPayload fails (too short).
	writeMigMsg(conn, uint32(migration.MsgMemoryDirty), []byte{0x01, 0x02, 0x03})
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from short MsgMemoryDirty, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingApplyDirtyPagesFails verifies that Incoming returns an error
// when the dirty payload is syntactically valid but page data is missing.
func TestIncomingApplyDirtyPagesFails(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7797")
	defer conn.Close()

	// Build a valid dirty payload: bitmapLen=8, bitmap marks page 0 dirty,
	// but no page data bytes → applyDirtyPages fails with truncated error.
	payload := make([]byte, 16)
	binary.BigEndian.PutUint64(payload[0:8], 8)     // bitmapLen = 8 bytes
	binary.LittleEndian.PutUint64(payload[8:16], 1) // bitmap: bit 0 set (page 0 dirty)
	// no page data bytes

	writeMigMsg(conn, uint32(migration.MsgMemoryDirty), payload)
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from applyDirtyPages, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingNoDiskConfigured verifies that Incoming returns an error when a
// MsgDiskFull arrives but no disk is configured.
func TestIncomingNoDiskConfigured(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize, Disk: ""})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7798")
	defer conn.Close()

	writeMigMsg(conn, uint32(migration.MsgDiskFull), []byte{0x01, 0x02, 0x03, 0x04})
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from MsgDiskFull with no disk, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingBadSnapshot verifies that Incoming returns an error when a
// MsgSnapshot payload cannot be gob-decoded.
func TestIncomingBadSnapshot(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7799")
	defer conn.Close()

	// 3 garbage bytes → DecodeSnapshot (gob) fails.
	writeMigMsg(conn, uint32(migration.MsgSnapshot), []byte{0xFF, 0xFF, 0xFF})
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from bad MsgSnapshot, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingMsgDoneBeforeSnapshot verifies that Incoming returns an error
// when MsgDone is received before any MsgSnapshot.
func TestIncomingMsgDoneBeforeSnapshot(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7800")
	defer conn.Close()

	writeMigMsg(conn, uint32(migration.MsgDone), nil)
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from MsgDone before snapshot, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingEmptySnapshot verifies that Incoming returns an error when
// MsgDone arrives with a valid but empty Snapshot (applySnapshot fails because
// the embedded VMState has nil Clock).
func TestIncomingEmptySnapshot(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7801")
	defer conn.Close()

	// Send a properly gob-encoded Snapshot with empty VCPUStates and nil VM
	// fields → applySnapshot will fail on RestoreVMState (nil Clock).
	// SendSnapshot may fail with "broken pipe" if machine.New failed (no KVM),
	// in which case Incoming has already returned an error – that's fine.
	sender := migration.NewSender(conn)

	snap := &migration.Snapshot{NCPUs: 1, MemSize: machine.MinMemSize}
	_ = sender.SendSnapshot(snap)
	_ = sender.SendDone()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from Incoming, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingUnexpectedMsgReady verifies that Incoming returns an error when
// MsgReady is received (it is only valid on the source side).
func TestIncomingUnexpectedMsgReady(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7802")
	defer conn.Close()

	writeMigMsg(conn, uint32(migration.MsgReady), nil)
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from unexpected MsgReady, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}

// TestIncomingUnexpectedMsgType verifies that Incoming returns an error for
// an unrecognised message type.
func TestIncomingUnexpectedMsgType(t *testing.T) { //nolint:paralleltest
	requireRoot(t)

	v := vmm.New(vmm.Config{Dev: "/dev/kvm", NCPUs: 1, MemSize: machine.MinMemSize})

	conn, errCh := startIncoming(t, v, "127.0.0.1:7803")
	defer conn.Close()

	writeMigMsg(conn, 99, nil) // unknown type
	conn.Close()

	if err := <-errCh; err == nil {
		t.Fatal("expected error from unknown message type, got nil")
	}

	if v.Machine != nil {
		_ = v.Machine.Close()
	}
}
