package virtio_test

import (
	"bytes"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/bobuhiro11/gokvm/virtio"
)

// countingInjector counts InjectVirtioBlkIRQ calls.
type countingInjector struct {
	blkCount atomic.Int64
}

func (c *countingInjector) InjectVirtioNetIRQ() error {
	return nil
}

func (c *countingInjector) InjectVirtioBlkIRQ() error {
	c.blkCount.Add(1)

	return nil
}

func TestBlkGetDeviceHeader(t *testing.T) {
	t.Parallel()

	v, err := virtio.NewBlk("/dev/zero", 9, &mockInjector{}, []byte{})
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	expected := uint16(0x1001)
	actual := v.GetDeviceHeader().DeviceID

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestBlkGetIORange(t *testing.T) {
	t.Parallel()

	v, err := virtio.NewBlk("/dev/zero", 9, &mockInjector{}, []byte{})
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	actual := v.Size()
	expected := uint64(virtio.BlkIOPortSize)

	if actual != expected {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestBlkIOInHandler(t *testing.T) {
	t.Parallel()

	v, err := virtio.NewBlk("/dev/zero", 9, &mockInjector{}, []byte{})
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	expected := []byte{0x20, 0x00}
	actual := make([]byte, 2)
	_ = v.Read(virtio.BlkIOPortStart+12, actual)

	if !bytes.Equal(expected, actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
}

func TestIO(t *testing.T) {
	t.Parallel()

	mem := make([]byte, 0x1000000)

	v, err := virtio.NewBlk(
		"../vda.img", 10, &mockInjector{}, mem,
	)

	if os.IsNotExist(err) {
		t.Skipf(
			"../vda.img does not exist, skipping",
		)
	}

	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	// Init virt queue
	vq := virtio.VirtQueue{}
	vq.AvailRing.Idx = 1

	// desc[0]: blk request header
	vq.DescTable[0].Addr = 0
	vq.DescTable[0].Len = 16
	vq.DescTable[0].Next = 1

	blkReq := (*virtio.BlkReq)(
		unsafe.Pointer(&mem[0]),
	)
	blkReq.Type = 0
	blkReq.Sector = 2

	// desc[1]: data buffer
	vq.DescTable[1].Addr = 0x400
	vq.DescTable[1].Len = 0x200
	vq.DescTable[1].Next = 2

	// desc[2]: status byte
	vq.DescTable[2].Addr = 0x700
	vq.DescTable[2].Len = 1

	mem[0x700] = 0xFF // poison status byte

	v.VirtQueue[0] = &vq

	if err := v.IO(); err != nil {
		t.Fatalf("err: %v\n", err)
	}

	if !v.IRQInjector.(*mockInjector).called {
		t.Fatalf("irqInjected = false\n")
	}

	// Verify status byte is VIRTIO_BLK_S_OK (0)
	if mem[0x700] != 0 {
		t.Fatalf(
			"status: expected 0, got %d",
			mem[0x700],
		)
	}

	expected := []byte{0x53, 0xef}
	actual := mem[0x438:0x43a]

	if !bytes.Equal(expected, actual) {
		t.Fatalf(
			"expected: %v, actual: %v",
			expected, actual,
		)
	}
}

func TestBlkIOStatusByte(t *testing.T) {
	t.Parallel()

	// Create a temp file with known content.
	f, err := os.CreateTemp("", "blk-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())

	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i & 0xFF)
	}

	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}

	f.Close()

	mem := make([]byte, 0x100000)

	v, err := virtio.NewBlk(
		f.Name(), 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	vq := virtio.VirtQueue{}
	vq.AvailRing.Idx = 1

	// desc[0]: BlkReq header (16 bytes)
	vq.DescTable[0].Addr = 0x1000
	vq.DescTable[0].Len = 16
	vq.DescTable[0].Next = 1

	blkReq := (*virtio.BlkReq)(
		unsafe.Pointer(&mem[0x1000]),
	)
	blkReq.Type = 0 // read
	blkReq.Sector = 0

	// desc[1]: data buffer (512 bytes)
	vq.DescTable[1].Addr = 0x2000
	vq.DescTable[1].Len = 512
	vq.DescTable[1].Next = 2

	// desc[2]: status byte
	vq.DescTable[2].Addr = 0x3000
	vq.DescTable[2].Len = 1

	mem[0x3000] = 0xB8 // poison like machine.New()

	v.VirtQueue[0] = &vq

	if err := v.IO(); err != nil {
		t.Fatal(err)
	}

	// Status must be VIRTIO_BLK_S_OK (0).
	if mem[0x3000] != 0 {
		t.Fatalf(
			"status: expected 0, got %d",
			mem[0x3000],
		)
	}

	// Data buffer must contain file contents.
	if !bytes.Equal(mem[0x2000:0x2000+512], data[:512]) {
		t.Fatal("data mismatch")
	}

	// usedRing.Idx must be incremented.
	if vq.UsedRing.Idx != 1 {
		t.Fatalf(
			"usedRing.Idx: expected 1, got %d",
			vq.UsedRing.Idx,
		)
	}

	if !v.IRQInjector.(*mockInjector).called {
		t.Fatal("IRQ not injected")
	}
}

func TestBlkClose(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp("", "blk-close-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())
	f.Close()

	mem := make([]byte, 0x10000)

	v, err := virtio.NewBlk(
		f.Name(), 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Close(); err != nil {
		t.Fatalf("Close: got %v, want nil", err)
	}

	// Second close should fail because the file
	// descriptor is already closed.
	if err := v.Close(); err == nil {
		t.Fatal("second Close: got nil, want error")
	}
}

func TestBlkIOThreadExitsOnClose(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp("", "blk-iothread-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())
	f.Close()

	mem := make([]byte, 0x10000)

	v, err := virtio.NewBlk(
		f.Name(), 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		v.IOThreadEntry()
	}()

	if err := v.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(
			"IOThreadEntry did not exit after Close",
		)
	}
}

func TestBlkWriteNonBlockingKick(t *testing.T) {
	t.Parallel()

	mem := make([]byte, 0x10000)

	v, err := virtio.NewBlk(
		"/dev/zero", 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	defer v.Close()

	// Write offset 16 twice rapidly. With a blocking
	// send on a size-1 channel the second call would
	// block the vCPU. Both must complete promptly.
	for i := 0; i < 2; i++ {
		done := make(chan struct{})

		go func() {
			_ = v.Write(
				virtio.BlkIOPortStart+16,
				[]byte{0x0, 0x0},
			)

			close(done)
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatalf(
				"Write #%d to offset 16 blocked", i,
			)
		}
	}
}

func TestBlkWriteAfterClose(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp("", "blk-wac-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())
	f.Close()

	mem := make([]byte, 0x10000)

	v, err := virtio.NewBlk(
		f.Name(), 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Close(); err != nil {
		t.Fatal(err)
	}

	// Write at offset 16 after Close must not panic.
	if err := v.Write(
		virtio.BlkIOPortStart+16,
		[]byte{0x0, 0x0},
	); err != nil {
		t.Fatal(err)
	}
}

func TestBlkConcurrentCloseAndWrite(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp("", "blk-ccw-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())
	f.Close()

	mem := make([]byte, 0x10000)

	v, err := virtio.NewBlk(
		f.Name(), 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		v.Close()
	}()

	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			_ = v.Write(
				virtio.BlkIOPortStart+16,
				[]byte{0x0, 0x0},
			)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Close+Write deadlocked")
	}
}

func TestBlkISRClearedOnRead(t *testing.T) {
	t.Parallel()

	// Create a temp file with enough data for one sector.
	f, err := os.CreateTemp("", "blk-isr-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())

	data := make([]byte, 1024)
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}

	f.Close()

	mem := make([]byte, 0x100000)

	v, err := virtio.NewBlk(
		f.Name(), 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Set up a virtqueue and perform one IO to set ISR.
	vq := virtio.VirtQueue{}
	vq.AvailRing.Idx = 1

	vq.DescTable[0].Addr = 0x1000
	vq.DescTable[0].Len = 16
	vq.DescTable[0].Next = 1

	blkReq := (*virtio.BlkReq)(
		unsafe.Pointer(&mem[0x1000]),
	)
	blkReq.Type = 0 // read
	blkReq.Sector = 0

	vq.DescTable[1].Addr = 0x2000
	vq.DescTable[1].Len = 512
	vq.DescTable[1].Next = 2

	vq.DescTable[2].Addr = 0x3000
	vq.DescTable[2].Len = 1

	v.VirtQueue[0] = &vq

	if err := v.IO(); err != nil {
		t.Fatal(err)
	}

	// First read of ISR (offset 19) must return 1.
	buf := make([]byte, 1)
	if err := v.Read(
		virtio.BlkIOPortStart+19, buf,
	); err != nil {
		t.Fatal(err)
	}

	if buf[0] != 1 {
		t.Fatalf(
			"first ISR read: got %d, want 1",
			buf[0],
		)
	}

	// Second read must return 0 (cleared on read).
	if err := v.Read(
		virtio.BlkIOPortStart+19, buf,
	); err != nil {
		t.Fatal(err)
	}

	if buf[0] != 0 {
		t.Fatalf(
			"second ISR read: got %d, want 0",
			buf[0],
		)
	}
}

func TestBlkIOThreadReInjectsIRQ(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp("", "blk-reinject-*")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(f.Name())

	data := make([]byte, 1024)
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}

	f.Close()

	mem := make([]byte, 0x100000)
	inj := &countingInjector{}

	v, err := virtio.NewBlk(
		f.Name(), 10, inj, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Set up a virtqueue and perform one IO to set ISR.
	vq := virtio.VirtQueue{}
	vq.AvailRing.Idx = 1

	vq.DescTable[0].Addr = 0x1000
	vq.DescTable[0].Len = 16
	vq.DescTable[0].Next = 1

	blkReq := (*virtio.BlkReq)(
		unsafe.Pointer(&mem[0x1000]),
	)
	blkReq.Type = 0
	blkReq.Sector = 0

	vq.DescTable[1].Addr = 0x2000
	vq.DescTable[1].Len = 512
	vq.DescTable[1].Next = 2

	vq.DescTable[2].Addr = 0x3000
	vq.DescTable[2].Len = 1

	v.VirtQueue[0] = &vq

	if err := v.IO(); err != nil {
		t.Fatal(err)
	}

	// IO() calls InjectVirtioBlkIRQ once.
	before := inj.blkCount.Load()

	// Start IOThreadEntry — the ticker should
	// re-inject because ISR is still set.
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		v.IOThreadEntry()
	}()

	// Wait long enough for several ticks (10ms each).
	time.Sleep(100 * time.Millisecond)

	after := inj.blkCount.Load()

	v.Close()
	wg.Wait()

	reinjections := after - before
	if reinjections < 2 {
		t.Fatalf(
			"expected >=2 re-injections, got %d",
			reinjections,
		)
	}
}

func TestBlkIONilQueue(t *testing.T) {
	t.Parallel()

	mem := make([]byte, 0x10000)

	v, err := virtio.NewBlk(
		"/dev/zero", 10, &mockInjector{}, mem,
	)
	if err != nil {
		t.Fatal(err)
	}

	// VirtQueue[0] is nil by default.
	err = v.IO()
	if err == nil {
		t.Fatal("expected error for nil VirtQueue")
	}
}
