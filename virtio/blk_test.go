package virtio_test

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/bobuhiro11/gokvm/virtio"
)

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
