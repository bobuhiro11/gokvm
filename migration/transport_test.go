package migration_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"reflect"
	"testing"

	"github.com/bobuhiro11/gokvm/migration"
)

// ---- helpers ----------------------------------------------------------------

// pipe returns a connected (Sender, Receiver) pair backed by an in-memory pipe.
func pipe() (*migration.Sender, *migration.Receiver) {
	pr, pw := io.Pipe()

	return migration.NewSender(pw), migration.NewReceiver(pr)
}

// mustNext calls recv.Next and fails the test on error.
func mustNext(t *testing.T, recv *migration.Receiver) (migration.MsgType, []byte) {
	t.Helper()

	msgType, payload, err := recv.Next()
	if err != nil {
		t.Fatalf("Receiver.Next: %v", err)
	}

	return msgType, payload
}

// ---- transport: zero-payload messages ---------------------------------------

func TestSendReceiveDone(t *testing.T) {
	t.Parallel()

	sender, recv := pipe()

	go func() {
		if err := sender.SendDone(); err != nil {
			t.Errorf("SendDone: %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgDone {
		t.Fatalf("got type %d, want MsgDone (%d)", msgType, migration.MsgDone)
	}

	if len(payload) != 0 {
		t.Fatalf("MsgDone should carry no payload, got %d bytes", len(payload))
	}
}

func TestSendReceiveReady(t *testing.T) {
	t.Parallel()

	sender, recv := pipe()

	go func() {
		if err := sender.SendReady(); err != nil {
			t.Errorf("SendReady: %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgReady {
		t.Fatalf("got type %d, want MsgReady (%d)", msgType, migration.MsgReady)
	}

	if len(payload) != 0 {
		t.Fatalf("MsgReady should carry no payload, got %d bytes", len(payload))
	}
}

// ---- transport: memory messages --------------------------------------------

func TestSendReceiveMemoryFull(t *testing.T) {
	t.Parallel()

	const memSize = 4096 * 3
	mem := make([]byte, memSize)

	for i := range mem {
		mem[i] = byte(i % 251)
	}

	sender, recv := pipe()

	go func() {
		if err := sender.SendMemoryFull(mem); err != nil {
			t.Errorf("SendMemoryFull: %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgMemoryFull {
		t.Fatalf("got type %d, want MsgMemoryFull (%d)", msgType, migration.MsgMemoryFull)
	}

	if !bytes.Equal(payload, mem) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(payload), len(mem))
	}
}

// TestSendReceiveDiskFull verifies that disk image bytes survive the wire
// encoding intact.
func TestSendReceiveDiskFull(t *testing.T) {
	t.Parallel()

	const diskSize = 4096 * 2
	disk := make([]byte, diskSize)

	for i := range disk {
		disk[i] = byte(i % 199)
	}

	sender, recv := pipe()

	go func() {
		if err := sender.SendDiskFull(disk); err != nil {
			t.Errorf("SendDiskFull: %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgDiskFull {
		t.Fatalf("got type %d, want MsgDiskFull (%d)", msgType, migration.MsgDiskFull)
	}

	if !bytes.Equal(payload, disk) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(payload), len(disk))
	}
}

func TestSendReceiveMemoryDirty(t *testing.T) {
	t.Parallel()

	// Two dirty pages at page 0 and page 2 (bitmap word = 0b0101 = 5).
	bitmap := []uint64{5}
	bitmapBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bitmapBytes, bitmap[0])

	page0 := bytes.Repeat([]byte{0xAA}, 4096)
	page2 := bytes.Repeat([]byte{0xBB}, 4096)
	pageData := make([]byte, 0, 8192)
	pageData = append(pageData, page0...)
	pageData = append(pageData, page2...)

	sender, recv := pipe()

	go func() {
		if err := sender.SendMemoryDirty(bitmapBytes, pageData); err != nil {
			t.Errorf("SendMemoryDirty: %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgMemoryDirty {
		t.Fatalf("got type %d, want MsgMemoryDirty (%d)", msgType, migration.MsgMemoryDirty)
	}

	gotBitmap, gotPageData, err := migration.DecodeDirtyPayload(payload)
	if err != nil {
		t.Fatalf("DecodeDirtyPayload: %v", err)
	}

	if !bytes.Equal(gotBitmap, bitmapBytes) {
		t.Fatalf("bitmap mismatch: got %x, want %x", gotBitmap, bitmapBytes)
	}

	if !bytes.Equal(gotPageData, pageData) {
		t.Fatalf("page data mismatch (len got=%d want=%d)", len(gotPageData), len(pageData))
	}
}

// ---- transport: snapshot message -------------------------------------------

// makeSnapshot returns a Snapshot with non-zero values in every field so that
// a round-trip test catches missing/swapped fields.
func makeSnapshot() *migration.Snapshot {
	cpu := migration.VCPUState{
		Regs:      []byte{0x01, 0x02, 0x03},
		Sregs:     []byte{0x04, 0x05},
		MSRs:      []migration.MSREntry{{Index: 0x10, Data: 0x20}, {Index: 0x30, Data: 0x40}},
		LAPIC:     []byte{0xAB},
		Events:    []byte{0xCD},
		MPState:   1,
		DebugRegs: []byte{0xEF},
		XCRS:      []byte{0xFF},
	}

	vm := migration.VMState{
		Clock:         []byte{0x11},
		IRQChipPIC0:   []byte{0x22},
		IRQChipPIC1:   []byte{0x33},
		IRQChipIOAPIC: []byte{0x44},
		PIT2:          []byte{0x55},
	}

	serial := migration.SerialState{IER: 0x0F, LCR: 0x03}

	blk := &migration.BlkState{
		HdrBytes:      []byte{0xBB, 0xCC},
		QueuePhysAddr: [1]uint64{0x1000},
		LastAvailIdx:  [1]uint16{7},
	}

	net := &migration.NetState{
		HdrBytes:      []byte{0xDD, 0xEE},
		QueuePhysAddr: [2]uint64{0x2000, 0x3000},
		LastAvailIdx:  [2]uint16{3, 5},
	}

	return &migration.Snapshot{
		NCPUs:      2,
		MemSize:    1 << 25,
		VCPUStates: []migration.VCPUState{cpu, cpu},
		VM:         vm,
		Devices:    migration.DeviceState{Serial: serial, Blk: blk, Net: net},
	}
}

func TestSendReceiveSnapshot(t *testing.T) {
	t.Parallel()

	snap := makeSnapshot()
	sender, recv := pipe()

	go func() {
		if err := sender.SendSnapshot(snap); err != nil {
			t.Errorf("SendSnapshot: %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgSnapshot {
		t.Fatalf("got type %d, want MsgSnapshot (%d)", msgType, migration.MsgSnapshot)
	}

	got, err := migration.DecodeSnapshot(payload)
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}

	if !reflect.DeepEqual(got, snap) {
		t.Fatalf("snapshot round-trip mismatch:\ngot  %+v\nwant %+v", got, snap)
	}
}

// ---- transport: full protocol sequence -------------------------------------

// TestFullMigrationProtocol sends the complete sequence of messages a real
// source would produce and verifies the receiver sees them in order.
func TestFullMigrationProtocol(t *testing.T) {
	t.Parallel()

	const pageSize = 4096

	const pages = 4

	mem := make([]byte, pageSize*pages)
	for i := range mem {
		mem[i] = byte(i)
	}

	// Dirty round: pages 1 and 3 (bitmap word = 0b1010 = 0xA).
	dirtyBitmapWord := uint64(0xA)
	bitmapBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bitmapBytes, dirtyBitmapWord)

	dirtyPage1 := bytes.Repeat([]byte{0x11}, pageSize)
	dirtyPage3 := bytes.Repeat([]byte{0x33}, pageSize)
	pageData := make([]byte, 0, pageSize*2)
	pageData = append(pageData, dirtyPage1...)
	pageData = append(pageData, dirtyPage3...)

	snap := makeSnapshot()

	disk := bytes.Repeat([]byte{0xDA}, pageSize*2)

	sender, recv := pipe()

	// Run sender in background.
	errc := make(chan error, 1)

	go func() {
		var err error

		if err = sender.SendMemoryFull(mem); err != nil {
			errc <- err

			return
		}

		if err = sender.SendMemoryDirty(bitmapBytes, pageData); err != nil {
			errc <- err

			return
		}

		if err = sender.SendDiskFull(disk); err != nil {
			errc <- err

			return
		}

		if err = sender.SendSnapshot(snap); err != nil {
			errc <- err

			return
		}

		err = sender.SendDone()
		errc <- err
	}()

	// Receive and verify each message in order.
	wantTypes := []migration.MsgType{
		migration.MsgMemoryFull,
		migration.MsgMemoryDirty,
		migration.MsgDiskFull,
		migration.MsgSnapshot,
		migration.MsgDone,
	}

	for _, wantType := range wantTypes {
		msgType, payload, err := recv.Next()
		if err != nil {
			t.Fatalf("recv.Next (want %d): %v", wantType, err)
		}

		if msgType != wantType {
			t.Fatalf("message order: got type %d, want %d", msgType, wantType)
		}

		switch msgType {
		case migration.MsgMemoryFull:
			if !bytes.Equal(payload, mem) {
				t.Fatalf("MsgMemoryFull payload mismatch")
			}

		case migration.MsgMemoryDirty:
			gb, gd, err := migration.DecodeDirtyPayload(payload)
			if err != nil {
				t.Fatalf("DecodeDirtyPayload: %v", err)
			}

			if !bytes.Equal(gb, bitmapBytes) {
				t.Fatalf("dirty bitmap mismatch: %x vs %x", gb, bitmapBytes)
			}

			if !bytes.Equal(gd, pageData) {
				t.Fatalf("dirty page data mismatch")
			}

		case migration.MsgDiskFull:
			if !bytes.Equal(payload, disk) {
				t.Fatalf("MsgDiskFull payload mismatch: got %d bytes, want %d", len(payload), len(disk))
			}

		case migration.MsgSnapshot:
			got, err := migration.DecodeSnapshot(payload)
			if err != nil {
				t.Fatalf("DecodeSnapshot: %v", err)
			}

			if !reflect.DeepEqual(got, snap) {
				t.Fatalf("snapshot mismatch")
			}

		case migration.MsgDone:
			if len(payload) != 0 {
				t.Fatalf("MsgDone should have no payload")
			}

		case migration.MsgReady:
			// Unexpected in this test but handled for completeness

		default:
			t.Fatalf("unexpected message type: %v", msgType)
		}
	}

	if err := <-errc; err != nil {
		t.Fatalf("sender goroutine: %v", err)
	}
}

// ---- DecodeDirtyPayload error cases ----------------------------------------

func TestDecodeDirtyPayloadTooShort(t *testing.T) {
	t.Parallel()

	_, _, err := migration.DecodeDirtyPayload([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for short payload, got nil")
	}
}

func TestDecodeDirtyPayloadTruncatedBitmap(t *testing.T) {
	t.Parallel()

	// Announce 100 bytes of bitmap but provide only 4.
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint64(hdr, 100)

	payload := make([]byte, 0, 12)
	payload = append(payload, hdr...)
	payload = append(payload, 0x01, 0x02, 0x03, 0x04)

	_, _, err := migration.DecodeDirtyPayload(payload)
	if err == nil {
		t.Fatal("expected error for truncated bitmap, got nil")
	}
}

func TestDecodeDirtyPayloadEmptyBitmap(t *testing.T) {
	t.Parallel()

	// Zero-length bitmap with non-empty page data.
	hdr := make([]byte, 8) // bitmapLen = 0
	payload := make([]byte, 0, 10)
	payload = append(payload, hdr...)
	payload = append(payload, 0xDE, 0xAD)

	bitmapBytes, pageData, err := migration.DecodeDirtyPayload(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bitmapBytes) != 0 {
		t.Fatalf("expected empty bitmap, got %d bytes", len(bitmapBytes))
	}

	if len(pageData) != 2 {
		t.Fatalf("expected 2 bytes of page data, got %d", len(pageData))
	}
}

// ---- DecodeSnapshot error cases --------------------------------------------

func TestDecodeSnapshotInvalidGob(t *testing.T) {
	t.Parallel()

	_, err := migration.DecodeSnapshot([]byte{0xFF, 0xFE, 0xFD})
	if err == nil {
		t.Fatal("expected error decoding garbage, got nil")
	}
}

// ---- Snapshot gob round-trip without transport -----------------------------

func TestSnapshotGobRoundTrip(t *testing.T) {
	t.Parallel()

	snap := makeSnapshot()

	// Encode via SendSnapshot internals (via Sender over a buffer).
	var buf bytes.Buffer
	sender := migration.NewSender(&buf)

	if err := sender.SendSnapshot(snap); err != nil {
		t.Fatalf("SendSnapshot: %v", err)
	}

	recv := migration.NewReceiver(&buf)

	msgType, payload, err := recv.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if msgType != migration.MsgSnapshot {
		t.Fatalf("got %d, want MsgSnapshot", msgType)
	}

	got, err := migration.DecodeSnapshot(payload)
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}

	// NCPUs / MemSize.
	if got.NCPUs != snap.NCPUs || got.MemSize != snap.MemSize {
		t.Fatalf("Snapshot metadata mismatch: got NCPUs=%d MemSize=%d, want NCPUs=%d MemSize=%d",
			got.NCPUs, got.MemSize, snap.NCPUs, snap.MemSize)
	}

	// VCPUStates length.
	if len(got.VCPUStates) != len(snap.VCPUStates) {
		t.Fatalf("VCPUStates length: got %d, want %d", len(got.VCPUStates), len(snap.VCPUStates))
	}

	for i := range snap.VCPUStates {
		if !reflect.DeepEqual(got.VCPUStates[i], snap.VCPUStates[i]) {
			t.Fatalf("VCPUState[%d] mismatch", i)
		}
	}

	// VMState.
	if !reflect.DeepEqual(got.VM, snap.VM) {
		t.Fatalf("VMState mismatch:\ngot  %+v\nwant %+v", got.VM, snap.VM)
	}

	// DeviceState.
	if got.Devices.Serial != snap.Devices.Serial {
		t.Fatalf("SerialState mismatch")
	}

	if !reflect.DeepEqual(got.Devices.Blk, snap.Devices.Blk) {
		t.Fatalf("BlkState mismatch:\ngot  %+v\nwant %+v", got.Devices.Blk, snap.Devices.Blk)
	}

	if !reflect.DeepEqual(got.Devices.Net, snap.Devices.Net) {
		t.Fatalf("NetState mismatch:\ngot  %+v\nwant %+v", got.Devices.Net, snap.Devices.Net)
	}
}

// TestSnapshotWithNilDevices verifies that a Snapshot with no blk/net device
// (Blk == nil, Net == nil) round-trips correctly.
func TestSnapshotWithNilDevices(t *testing.T) {
	t.Parallel()

	snap := &migration.Snapshot{
		NCPUs:   1,
		MemSize: 1 << 25,
		VCPUStates: []migration.VCPUState{
			{Regs: []byte{1, 2}, MPState: 0},
		},
		VM: migration.VMState{Clock: []byte{0xAA}},
		Devices: migration.DeviceState{
			Serial: migration.SerialState{IER: 0x01, LCR: 0x02},
			Blk:    nil,
			Net:    nil,
		},
	}

	var buf bytes.Buffer
	sender := migration.NewSender(&buf)

	if err := sender.SendSnapshot(snap); err != nil {
		t.Fatalf("SendSnapshot: %v", err)
	}

	recv := migration.NewReceiver(&buf)

	_, payload, err := recv.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	got, err := migration.DecodeSnapshot(payload)
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}

	if got.Devices.Blk != nil {
		t.Fatal("expected nil Blk after round-trip")
	}

	if got.Devices.Net != nil {
		t.Fatal("expected nil Net after round-trip")
	}
}

// TestMultipleMessages verifies that multiple messages sent over the same
// connection are demultiplexed correctly.
func TestMultipleMessages(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	sender := migration.NewSender(&buf)
	recv := migration.NewReceiver(&buf)

	// Write all messages first, then read them back (synchronous – no goroutines needed).
	_ = sender.SendReady()
	_ = sender.SendDone()
	_ = sender.SendMemoryFull([]byte{1, 2, 3})

	for i, wantType := range []migration.MsgType{
		migration.MsgReady,
		migration.MsgDone,
		migration.MsgMemoryFull,
	} {
		msgType, _, err := recv.Next()
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}

		if msgType != wantType {
			t.Fatalf("message %d: got type %d, want %d", i, msgType, wantType)
		}
	}
}

// TestReceiverEOF verifies that Next returns an error when the stream is closed
// before a full header is delivered.
func TestReceiverEOF(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer // empty

	recv := migration.NewReceiver(&buf)
	_, _, err := recv.Next()

	if err == nil {
		t.Fatal("expected error on empty stream, got nil")
	}
}

// TestReceiverTruncatedHeader verifies that Next returns an error when the
// stream ends in the middle of a 12-byte header.
func TestReceiverTruncatedHeader(t *testing.T) {
	t.Parallel()

	// Write only 6 bytes (less than the 12-byte header).
	var buf bytes.Buffer

	buf.Write([]byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00})

	recv := migration.NewReceiver(&buf)
	_, _, err := recv.Next()

	if err == nil {
		t.Fatal("expected error for truncated header, got nil")
	}
}

// TestReceiverTruncatedPayload verifies that Next returns an error when the
// header claims N bytes of payload but fewer are available in the stream.
func TestReceiverTruncatedPayload(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	// Header: type=MsgMemoryFull (2), length=1000
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(migration.MsgMemoryFull))
	binary.BigEndian.PutUint64(hdr[4:12], 1000)
	buf.Write(hdr)
	buf.Write([]byte{0x01, 0x02, 0x03}) // only 3 bytes instead of 1000

	recv := migration.NewReceiver(&buf)
	_, _, err := recv.Next()

	if err == nil {
		t.Fatal("expected error for truncated payload, got nil")
	}
}

// TestSendMemoryFullEmpty verifies that an empty memory slice is transported
// without error and that the receiver sees a zero-length payload.
func TestSendMemoryFullEmpty(t *testing.T) {
	t.Parallel()

	sender, recv := pipe()

	go func() {
		if err := sender.SendMemoryFull([]byte{}); err != nil {
			t.Errorf("SendMemoryFull(empty): %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgMemoryFull {
		t.Fatalf("got type %d, want MsgMemoryFull", msgType)
	}

	if len(payload) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(payload))
	}
}

// TestSendMemoryDirtyEmptyInputs verifies that SendMemoryDirty with nil bitmap
// and nil page data round-trips without error.
func TestSendMemoryDirtyEmptyInputs(t *testing.T) {
	t.Parallel()

	sender, recv := pipe()

	go func() {
		if err := sender.SendMemoryDirty(nil, nil); err != nil {
			t.Errorf("SendMemoryDirty(nil,nil): %v", err)
		}
	}()

	msgType, payload := mustNext(t, recv)

	if msgType != migration.MsgMemoryDirty {
		t.Fatalf("got type %d, want MsgMemoryDirty", msgType)
	}

	bitmapBytes, pageData, err := migration.DecodeDirtyPayload(payload)
	if err != nil {
		t.Fatalf("DecodeDirtyPayload: %v", err)
	}

	if len(bitmapBytes) != 0 {
		t.Fatalf("expected empty bitmap, got %d bytes", len(bitmapBytes))
	}

	if len(pageData) != 0 {
		t.Fatalf("expected empty page data, got %d bytes", len(pageData))
	}
}
