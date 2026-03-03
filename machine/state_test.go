package machine_test

// state_test.go – tests for machine/state.go migration helpers.
//
// All tests here require /dev/kvm (root inside a user namespace is sufficient).
// They are guarded by an os.Getuid()==0 check that mirrors the guard used in
// the rest of the machine test suite.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/migration"
)

// newTestMachine creates a minimal KVM machine for state tests.
// It returns the machine and a cleanup function.
func newTestMachine(t *testing.T) *machine.Machine {
	t.Helper()

	if os.Getuid() != 0 {
		t.Skip("state tests require root (run inside unshare --user --net --map-root-user)")
	}

	m, err := machine.New("/dev/kvm", 1, machine.MinMemSize)
	if err != nil {
		t.Fatalf("machine.New: %v", err)
	}

	t.Cleanup(func() { m.Close() })

	return m
}

// TestSaveCPUStateRoundTrip verifies that SaveCPUState succeeds on a fresh
// vCPU and that the saved state can be restored to a second machine without
// error.
func TestSaveCPUStateRoundTrip(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	state, err := src.SaveCPUState(0)
	if err != nil {
		t.Fatalf("SaveCPUState: %v", err)
	}

	if state == nil {
		t.Fatal("SaveCPUState returned nil state")
	}

	if len(state.Regs) == 0 {
		t.Error("Regs empty")
	}

	if len(state.Sregs) == 0 {
		t.Error("Sregs empty")
	}

	// Restore onto a second machine.
	dst := newTestMachine(t)
	if err := dst.RestoreCPUState(0, state); err != nil {
		t.Fatalf("RestoreCPUState: %v", err)
	}
}

// TestSaveVMStateRoundTrip verifies SaveVMState and RestoreVMState succeed
// on fresh machines.
func TestSaveVMStateRoundTrip(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	vmState, err := src.SaveVMState()
	if err != nil {
		t.Fatalf("SaveVMState: %v", err)
	}

	if vmState == nil {
		t.Fatal("SaveVMState returned nil")
	}

	if len(vmState.Clock) == 0 {
		t.Error("Clock bytes empty")
	}

	// Restore onto a fresh machine.
	dst := newTestMachine(t)
	if err := dst.RestoreVMState(vmState); err != nil {
		t.Fatalf("RestoreVMState: %v", err)
	}
}

// TestSaveDeviceStateNoDevices verifies SaveDeviceState works on a machine
// that has no virtio devices attached (serial is nil until Init* is called).
func TestSaveDeviceStateNoDevices(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	ds, err := m.SaveDeviceState()
	if err != nil {
		t.Fatalf("SaveDeviceState: %v", err)
	}

	if ds == nil {
		t.Fatal("SaveDeviceState returned nil")
	}

	// RestoreDeviceState on a machine with no devices should be a no-op.
	if err := m.RestoreDeviceState(ds); err != nil {
		t.Fatalf("RestoreDeviceState: %v", err)
	}
}

// TestSaveRestoreMemory verifies that SaveMemory and RestoreMemory
// round-trip the full guest physical memory correctly.
func TestSaveRestoreMemory(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	// Write a known pattern into guest memory.
	mem := src.Mem()
	for i := range mem {
		mem[i] = byte(i % 251)
	}

	// Save.
	var buf bytes.Buffer
	if err := src.SaveMemory(&buf); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if buf.Len() != len(mem) {
		t.Fatalf("saved %d bytes, want %d", buf.Len(), len(mem))
	}

	// Restore into a second machine.
	dst := newTestMachine(t)

	if err := dst.RestoreMemory(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("RestoreMemory: %v", err)
	}

	if !bytes.Equal(dst.Mem(), mem) {
		t.Error("restored memory does not match original")
	}
}

// TestEnableDirtyTrackingAndBitmap verifies EnableDirtyTracking and
// GetAndClearDirtyBitmap succeed, and that the bitmap has the expected size.
func TestEnableDirtyTrackingAndBitmap(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	if err := m.EnableDirtyTracking(); err != nil {
		t.Fatalf("EnableDirtyTracking: %v", err)
	}

	bitmap, err := m.GetAndClearDirtyBitmap()
	if err != nil {
		t.Fatalf("GetAndClearDirtyBitmap: %v", err)
	}

	pageSize := 4096
	memLen := len(m.Mem())
	numPages := (memLen + pageSize - 1) / pageSize
	wantWords := (numPages + 63) / 64

	if len(bitmap) != wantWords {
		t.Errorf("bitmap len %d, want %d", len(bitmap), wantWords)
	}
}

// TestTransferDirtyPages verifies that TransferDirtyPages writes exactly the
// pages marked in the bitmap.
func TestTransferDirtyPages(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	// Mark page 0 and page 1 as dirty (bitmap word = 0b11 = 3).
	// Write recognisable data to those pages first.
	const pageSize = 4096

	copy(m.Mem()[0:], bytes.Repeat([]byte{0xAA}, pageSize))
	copy(m.Mem()[pageSize:], bytes.Repeat([]byte{0xBB}, pageSize))

	bitmap := make([]uint64, (len(m.Mem())/pageSize+63)/64)
	bitmap[0] = 3 // bits 0 and 1 set

	var buf bytes.Buffer

	count, err := m.TransferDirtyPages(&buf, bitmap)
	if err != nil {
		t.Fatalf("TransferDirtyPages: %v", err)
	}

	if count != 2 {
		t.Errorf("transferred %d pages, want 2", count)
	}

	if buf.Len() != 2*pageSize {
		t.Errorf("wrote %d bytes, want %d", buf.Len(), 2*pageSize)
	}

	data := buf.Bytes()
	for i := 0; i < pageSize; i++ {
		if data[i] != 0xAA {
			t.Fatalf("page0 byte %d = %#x, want 0xAA", i, data[i])
		}
	}

	for i := pageSize; i < 2*pageSize; i++ {
		if data[i] != 0xBB {
			t.Fatalf("page1 byte %d = %#x, want 0xBB", i, data[i])
		}
	}
}

// TestTransferDirtyPagesNone verifies that TransferDirtyPages with an all-zero
// bitmap transfers no bytes.
func TestTransferDirtyPagesNone(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	bitmap := make([]uint64, (len(m.Mem())/4096+63)/64)
	// all zeros – no dirty pages

	var buf bytes.Buffer

	count, err := m.TransferDirtyPages(&buf, bitmap)
	if err != nil {
		t.Fatalf("TransferDirtyPages (no dirty): %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 pages transferred, got %d", count)
	}

	if buf.Len() != 0 {
		t.Errorf("expected 0 bytes written, got %d", buf.Len())
	}
}

// TestMsrIndexListCacheHit verifies that calling SaveCPUState twice reuses
// the cached MSR index list (the second call must also succeed).
func TestMsrIndexListCacheHit(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	s1, err := m.SaveCPUState(0)
	if err != nil {
		t.Fatalf("first SaveCPUState: %v", err)
	}

	s2, err := m.SaveCPUState(0)
	if err != nil {
		t.Fatalf("second SaveCPUState (cache hit): %v", err)
	}

	// Both calls should capture the same number of MSRs.
	if len(s1.MSRs) != len(s2.MSRs) {
		t.Errorf("MSR count mismatch: first=%d second=%d", len(s1.MSRs), len(s2.MSRs))
	}
}

// TestRestoreMemoryShortRead verifies that RestoreMemory returns an error when
// the reader yields fewer bytes than the machine memory size.
func TestRestoreMemoryShortRead(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	short := io.LimitReader(bytes.NewReader(make([]byte, 1024)), 1024)

	if err := m.RestoreMemory(short); err == nil {
		t.Fatal("expected error for short read, got nil")
	}
}

// TestSaveCPUStateInvalidCPU verifies that SaveCPUState returns an error when
// the cpu index is out of range (CPUToFD fails).
func TestSaveCPUStateInvalidCPU(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	if _, err := m.SaveCPUState(99); err == nil {
		t.Fatal("expected error for cpu=99, got nil")
	}
}

// TestRestoreVMStateDecodeClock verifies that RestoreVMState returns an error
// when the Clock field bytes are nil (too few bytes for kvm.ClockData).
func TestRestoreVMStateDecodeClock(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	if err := m.RestoreVMState(&migration.VMState{}); err == nil {
		t.Fatal("expected error from nil Clock, got nil")
	}
}

// TestRestoreVMStateDecodeIRQChip verifies an error when IRQChipPIC0 is nil
// but Clock bytes are valid (SetClock succeeds, IRQChip decode fails).
func TestRestoreVMStateDecodeIRQChip(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	vmState, err := src.SaveVMState()
	if err != nil {
		t.Fatalf("SaveVMState: %v", err)
	}

	dst := newTestMachine(t)

	// Valid clock but nil PIC0 → decode IRQChip fails.
	if err := dst.RestoreVMState(&migration.VMState{Clock: vmState.Clock}); err == nil {
		t.Fatal("expected error for nil IRQChipPIC0, got nil")
	}
}

// TestRestoreVMStateDecodePIT2 verifies an error when PIT2 is nil but Clock
// and all IRQChip bytes are valid.
func TestRestoreVMStateDecodePIT2(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	vmState, err := src.SaveVMState()
	if err != nil {
		t.Fatalf("SaveVMState: %v", err)
	}

	dst := newTestMachine(t)

	// Valid clock + IRQChips but nil PIT2 → decode PIT2 fails.
	bad := &migration.VMState{
		Clock:         vmState.Clock,
		IRQChipPIC0:   vmState.IRQChipPIC0,
		IRQChipPIC1:   vmState.IRQChipPIC1,
		IRQChipIOAPIC: vmState.IRQChipIOAPIC,
	}

	if err := dst.RestoreVMState(bad); err == nil {
		t.Fatal("expected error for nil PIT2, got nil")
	}
}

// TestRestoreCPUStateInvalidCPU verifies CPUToFD returns an error for an
// out-of-range CPU index.
func TestRestoreCPUStateInvalidCPU(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	if err := m.RestoreCPUState(-1, &migration.VCPUState{}); err == nil {
		t.Fatal("expected error for cpu=-1, got nil")
	}
}

// TestRestoreCPUStateNilFields exercises each copyStruct decode error path in
// RestoreCPUState by leaving one field nil while providing valid bytes for
// all preceding fields.
func TestRestoreCPUStateNilFields(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	state, err := src.SaveCPUState(0)
	if err != nil {
		t.Fatalf("SaveCPUState: %v", err)
	}

	dst := newTestMachine(t)

	// Nil Regs → decode Regs fails.
	if err := dst.RestoreCPUState(0, &migration.VCPUState{}); err == nil {
		t.Error("expected error for nil Regs, got nil")
	}

	// Nil Sregs → decode Sregs fails (Regs valid).
	if err := dst.RestoreCPUState(0, &migration.VCPUState{
		Regs: state.Regs,
	}); err == nil {
		t.Error("expected error for nil Sregs, got nil")
	}

	// Nil LAPIC → decode LAPIC fails (Regs, Sregs, MSRs valid).
	if err := dst.RestoreCPUState(0, &migration.VCPUState{
		Regs: state.Regs, Sregs: state.Sregs, MSRs: state.MSRs,
	}); err == nil {
		t.Error("expected error for nil LAPIC, got nil")
	}

	// Nil Events → decode Events fails (Regs through LAPIC valid).
	if err := dst.RestoreCPUState(0, &migration.VCPUState{
		Regs: state.Regs, Sregs: state.Sregs, MSRs: state.MSRs,
		LAPIC: state.LAPIC,
	}); err == nil {
		t.Error("expected error for nil Events, got nil")
	}

	// Nil DebugRegs → decode DebugRegs fails (Regs through Events valid).
	if err := dst.RestoreCPUState(0, &migration.VCPUState{
		Regs: state.Regs, Sregs: state.Sregs, MSRs: state.MSRs,
		LAPIC: state.LAPIC, Events: state.Events,
	}); err == nil {
		t.Error("expected error for nil DebugRegs, got nil")
	}

	// Nil XCRS → decode XCRS fails (Regs through DebugRegs valid).
	if err := dst.RestoreCPUState(0, &migration.VCPUState{
		Regs: state.Regs, Sregs: state.Sregs, MSRs: state.MSRs,
		LAPIC: state.LAPIC, Events: state.Events, DebugRegs: state.DebugRegs,
	}); err == nil {
		t.Error("expected error for nil XCRS, got nil")
	}
}

// errWriter is an io.Writer that always returns an error.
type errWriter struct{}

var errSimulatedWrite = errors.New("simulated write error")

func (errWriter) Write([]byte) (int, error) {
	return 0, errSimulatedWrite
}

// TestTransferDirtyPagesOutOfRange verifies that TransferDirtyPages skips
// (breaks) when a set bitmap bit would map to a page beyond guest memory.
func TestTransferDirtyPagesOutOfRange(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	// MinMemSize = 32 MiB → 8192 pages → 128 bitmap words.
	// Adding word 128 with bit 0 set → page 8192 → offset = 32 MiB → out of range.
	bitmap := make([]uint64, 129)
	bitmap[128] = 1

	count, err := m.TransferDirtyPages(&bytes.Buffer{}, bitmap)
	if err != nil {
		t.Fatalf("TransferDirtyPages: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 pages transferred (out-of-range skipped), got %d", count)
	}
}

// TestTransferDirtyPagesWriteError verifies that TransferDirtyPages propagates
// a writer error when a dirty page cannot be written.
func TestTransferDirtyPagesWriteError(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	// Mark page 0 as dirty.
	bitmap := make([]uint64, (len(m.Mem())/4096+63)/64)
	bitmap[0] = 1

	_, err := m.TransferDirtyPages(errWriter{}, bitmap)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// TestSaveRestoreDeviceStateWithDevices exercises the serial, Net, and Blk
// branches of SaveDeviceState and RestoreDeviceState using a machine that has
// all three device types attached.
func TestSaveRestoreDeviceStateWithDevices(t *testing.T) { //nolint:paralleltest
	m := newTestMachine(t)

	// InitForMigration attaches serial.
	if err := m.InitForMigration(); err != nil {
		t.Fatalf("InitForMigration: %v", err)
	}

	// AddTapIf attaches a virtio-net device.
	if err := m.AddTapIf("tap-state-test"); err != nil {
		t.Fatalf("AddTapIf: %v", err)
	}

	// AddDisk attaches a virtio-blk device.
	tmp, err := os.CreateTemp(t.TempDir(), "disk*.img")
	if err != nil {
		t.Fatalf("create tmp disk: %v", err)
	}

	// Minimum disk size for virtio-blk (at least one sector).
	if err := tmp.Truncate(512); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	tmp.Close()

	if err := m.AddDisk(tmp.Name()); err != nil {
		t.Fatalf("AddDisk: %v", err)
	}

	// SaveDeviceState should capture serial, net, and blk state.
	ds, err := m.SaveDeviceState()
	if err != nil {
		t.Fatalf("SaveDeviceState: %v", err)
	}

	if ds.Net == nil {
		t.Error("SaveDeviceState: Net state is nil")
	}

	if ds.Blk == nil {
		t.Error("SaveDeviceState: Blk state is nil")
	}

	// RestoreDeviceState should apply all non-nil device states.
	if err := m.RestoreDeviceState(ds); err != nil {
		t.Fatalf("RestoreDeviceState: %v", err)
	}
}

// TestSaveAndRestoreFullState is an integration test that exercises the
// complete Save*/Restore* path on a pair of fresh machines.
func TestSaveAndRestoreFullState(t *testing.T) { //nolint:paralleltest
	src := newTestMachine(t)

	// Write a marker into guest memory so we can verify RestoreMemory.
	const marker = uint64(0xDEADBEEFCAFEBABE)

	binary.LittleEndian.PutUint64(src.Mem()[0:8], marker)

	cpuState, err := src.SaveCPUState(0)
	if err != nil {
		t.Fatalf("SaveCPUState: %v", err)
	}

	vmState, err := src.SaveVMState()
	if err != nil {
		t.Fatalf("SaveVMState: %v", err)
	}

	devState, err := src.SaveDeviceState()
	if err != nil {
		t.Fatalf("SaveDeviceState: %v", err)
	}

	var memBuf bytes.Buffer
	if err := src.SaveMemory(&memBuf); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Restore onto a fresh machine.
	dst := newTestMachine(t)

	if err := dst.RestoreMemory(bytes.NewReader(memBuf.Bytes())); err != nil {
		t.Fatalf("RestoreMemory: %v", err)
	}

	if err := dst.RestoreCPUState(0, cpuState); err != nil {
		t.Fatalf("RestoreCPUState: %v", err)
	}

	if err := dst.RestoreVMState(vmState); err != nil {
		t.Fatalf("RestoreVMState: %v", err)
	}

	if err := dst.RestoreDeviceState(devState); err != nil {
		t.Fatalf("RestoreDeviceState: %v", err)
	}

	// Verify memory marker survived the round-trip.
	got := binary.LittleEndian.Uint64(dst.Mem()[0:8])
	if got != marker {
		t.Errorf("memory marker: got %#x, want %#x", got, marker)
	}
}
