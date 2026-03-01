package vmm

// migrate.go – live migration: source (MigrateTo) and destination (Incoming).
//
// Source side (MigrateTo):
//  1. Enable dirty-page tracking.
//  2. Pre-copy loop: send full memory, then up to maxPreCopyRounds rounds of
//     dirty pages while the VM runs.  Stop when dirty pages are below
//     preCopyThreshold or the maximum rounds are reached.
//  3. Pause all vCPUs (atomic stop flag + ImmediateExit).
//  4. Send one final dirty-page round.
//  5. Collect and send a Snapshot (CPU state, VM state, device state).
//  6. Send MsgDone and wait for MsgReady from the destination.
//  7. Terminate.
//
// Destination side (Incoming):
//  1. Allocate a machine with the same parameters (no kernel load).
//  2. Accept the TCP connection.
//  3. Receive memory (full + dirty rounds).
//  4. Receive and apply the Snapshot.
//  5. Send MsgReady.
//  6. Start vCPU goroutines.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/migration"
	"golang.org/x/sync/errgroup"
)

const (
	// maxPreCopyRounds is the maximum number of dirty-page iterations
	// before the VM is paused for the final transfer.
	maxPreCopyRounds = 3

	// preCopyThreshold is the fraction of total pages below which we
	// stop pre-copying and proceed to the pause-and-finalize step.
	preCopyThreshold = 0.01
)

var (
	errExpectedMsgReady      = errors.New("expected MsgReady")
	errMsgDoneBeforeSnapshot = errors.New("received MsgDone before Snapshot")
	errUnexpectedMessageType = errors.New("unexpected message type")
	errBitmapLengthNotMult8  = errors.New("bitmap length not a multiple of 8")
	errPageDataTruncated     = errors.New("page data truncated")
	errNoDiskConfigured      = errors.New("received disk data but no disk configured")
)

// controlSocketPath returns the Unix socket path for the given PID.
func controlSocketPath(pid int) string {
	return fmt.Sprintf("/tmp/gokvm-%d.sock", pid)
}

// StartControlSocket listens on a Unix domain socket and handles control
// commands sent by the `gokvm migrate` subcommand.
//
// Currently supported commands (newline-terminated):
//
//	MIGRATE <addr>   – trigger live migration to <addr> (host:port)
func (v *VMM) StartControlSocket() (string, error) {
	path := controlSocketPath(os.Getpid())

	l, err := net.Listen("unix", path)
	if err != nil {
		return "", fmt.Errorf("control socket: %w", err)
	}

	go func() {
		defer os.Remove(path)

		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			go v.handleControl(conn)
		}
	}()

	return path, nil
}

func (v *VMM) handleControl(conn net.Conn) {
	defer conn.Close()

	buf := new(strings.Builder)

	tmp := make([]byte, 256)

	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}

		if err != nil {
			break
		}

		if strings.Contains(buf.String(), "\n") {
			break
		}
	}

	line := strings.TrimSpace(buf.String())

	if strings.HasPrefix(line, "MIGRATE ") {
		addr := strings.TrimPrefix(line, "MIGRATE ")
		addr = strings.TrimSpace(addr)

		if err := v.MigrateTo(addr); err != nil {
			log.Printf("migration to %q failed: %v", addr, err)
			_, _ = conn.Write([]byte("ERROR " + err.Error() + "\n"))
		} else {
			_, _ = conn.Write([]byte("OK\n"))
		}
	} else {
		_, _ = conn.Write([]byte("ERROR unknown command\n"))
	}
}

// MigrateTo performs a live migration of the running VM to the given TCP
// address (host:port).  The source VM is paused for only the final state
// transfer; memory is streamed to the destination while the VM runs.
func (v *VMM) MigrateTo(addr string) error {
	log.Printf("migration: connecting to %s", addr)

	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	defer conn.Close()

	sender := migration.NewSender(conn)

	// Step 1: enable dirty-page tracking on the guest memory.
	if err := v.EnableDirtyTracking(); err != nil {
		return fmt.Errorf("EnableDirtyTracking: %w", err)
	}

	totalPages := len(v.Machine.Mem()) / 4096

	// Step 2a: send the full memory (first pre-copy pass).
	log.Printf("migration: sending full memory (%d MiB)", len(v.Machine.Mem())>>20)

	if err := sender.SendMemoryFull(v.Machine.Mem()); err != nil {
		return fmt.Errorf("SendMemoryFull: %w", err)
	}

	// Step 2b: iterative dirty-page rounds.
	for round := 0; round < maxPreCopyRounds; round++ {
		bitmap, err := v.GetAndClearDirtyBitmap()
		if err != nil {
			return err
		}

		// Count dirty pages.
		dirty := 0
		for _, w := range bitmap {
			dirty += popcount(w)
		}

		log.Printf("migration: pre-copy round %d: %d dirty pages", round+1, dirty)

		if dirty == 0 || float64(dirty)/float64(totalPages) < preCopyThreshold {
			break
		}

		bitmapBytes, pageData, err := collectDirtyPages(v.Machine, bitmap)
		if err != nil {
			return err
		}

		if err := sender.SendMemoryDirty(bitmapBytes, pageData); err != nil {
			return fmt.Errorf("SendMemoryDirty round %d: %w", round+1, err)
		}
	}

	// Step 3: pause all vCPUs and wait for them to actually stop so that
	// register reads below are not racing with KVM_RUN.
	log.Printf("migration: pausing vCPUs")
	v.Machine.PauseAndWait()

	// Step 3b: quiesce all I/O device threads so they cannot write to guest
	// memory after we take the final dirty-page snapshot.  This also ensures
	// GetState is not racing with the background threads.
	log.Printf("migration: quiescing I/O devices")
	v.Machine.QuiesceDevices()

	// Step 3c: send disk image if present (after quiesce so all writes are
	// flushed and the file descriptor is closed by the block device).
	if v.Disk != "" {
		log.Printf("migration: sending disk image %s", v.Disk)

		diskData, err := os.ReadFile(v.Disk)
		if err != nil {
			return fmt.Errorf("read disk %s: %w", v.Disk, err)
		}

		if err := sender.SendDiskFull(diskData); err != nil {
			return fmt.Errorf("SendDiskFull: %w", err)
		}

		log.Printf("migration: disk image sent (%d MiB)", len(diskData)>>20)
	}

	// Step 4: final dirty-page pass after pause (captures any writes made by
	// I/O threads between the pre-copy rounds and quiesce).
	bitmap, err := v.GetAndClearDirtyBitmap()
	if err != nil {
		return err
	}

	bitmapBytes, pageData, err := collectDirtyPages(v.Machine, bitmap)
	if err != nil {
		return err
	}

	if len(pageData) > 0 {
		if err := sender.SendMemoryDirty(bitmapBytes, pageData); err != nil {
			return fmt.Errorf("SendMemoryDirty final: %w", err)
		}
	}

	// Step 5: collect and send the VM snapshot.
	snap, err := buildSnapshot(v)
	if err != nil {
		return err
	}

	if err := sender.SendSnapshot(snap); err != nil {
		return fmt.Errorf("SendSnapshot: %w", err)
	}

	// Step 6: signal done and wait for destination acknowledgement.
	if err := sender.SendDone(); err != nil {
		return err
	}

	recv := migration.NewReceiver(conn)

	t, _, err := recv.Next()
	if err != nil {
		return fmt.Errorf("waiting for MsgReady: %w", err)
	}

	if t != migration.MsgReady {
		return fmt.Errorf("%w: got %v", errExpectedMsgReady, t)
	}

	log.Printf("migration: complete – destination is running")

	// Step 7: terminate the source.
	v.Machine.Close()

	return nil
}

// Incoming listens on listenAddr for an incoming migration and, once
// the full VM state is received, starts running the VM.
func (v *VMM) Incoming(listenAddr string) error {
	log.Printf("migration: waiting for incoming connection on %s", listenAddr)

	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listenAddr, err)
	}

	defer l.Close()

	conn, err := l.Accept()
	if err != nil {
		return fmt.Errorf("accept: %w", err)
	}

	defer conn.Close()

	// Allocate the machine (no kernel load – state comes from the source).
	m, err := machine.New(v.Dev, v.NCPUs, v.MemSize)
	if err != nil {
		return fmt.Errorf("machine.New: %w", err)
	}

	if len(v.TapIfName) > 0 {
		if err := m.AddTapIf(v.TapIfName); err != nil {
			return fmt.Errorf("AddTapIf: %w", err)
		}
	}

	if len(v.Disk) > 0 {
		if err := m.AddDisk(v.Disk); err != nil {
			return fmt.Errorf("AddDisk: %w", err)
		}
	}

	v.Machine = m

	// Initialise serial and IO-port handlers (normally done by LoadLinux/LoadPVH).
	if err := m.InitForMigration(); err != nil {
		return fmt.Errorf("InitForMigration: %w", err)
	}

	recv := migration.NewReceiver(conn)
	sender := migration.NewSender(conn)

	var snap *migration.Snapshot

	for {
		msgType, payload, err := recv.Next()
		if err != nil {
			return fmt.Errorf("receive: %w", err)
		}

		switch msgType {
		case migration.MsgMemoryFull:
			log.Printf("migration: receiving full memory (%d MiB)", len(payload)>>20)

			if err := m.RestoreMemory(bytes.NewReader(payload)); err != nil {
				return fmt.Errorf("RestoreMemory: %w", err)
			}

		case migration.MsgMemoryDirty:
			bitmapBytes, pageData, err := migration.DecodeDirtyPayload(payload)
			if err != nil {
				return err
			}

			if err := applyDirtyPages(m, bitmapBytes, pageData); err != nil {
				return fmt.Errorf("applyDirtyPages: %w", err)
			}

		case migration.MsgDiskFull:
			if v.Disk == "" {
				return errNoDiskConfigured
			}

			log.Printf("migration: receiving disk image (%d MiB)", len(payload)>>20)

			if err := os.WriteFile(v.Disk, payload, 0o600); err != nil {
				return fmt.Errorf("write disk %s: %w", v.Disk, err)
			}

		case migration.MsgSnapshot:
			snap, err = migration.DecodeSnapshot(payload)
			if err != nil {
				return err
			}

		case migration.MsgDone:
			if snap == nil {
				return fmt.Errorf("%w", errMsgDoneBeforeSnapshot)
			}

			if err := applySnapshot(m, snap); err != nil {
				return fmt.Errorf("applySnapshot: %w", err)
			}

			if err := sender.SendReady(); err != nil {
				return err
			}

			log.Printf("migration: state restored, starting VM")

			return v.runRestoredVM()

		case migration.MsgReady:
			// Not expected on destination side
			return fmt.Errorf("%w: %v", errUnexpectedMessageType, msgType)

		default:
			return fmt.Errorf("%w: %v", errUnexpectedMessageType, msgType)
		}
	}
}

// runRestoredVM starts vCPU goroutines for a VM that was restored from
// migration state (i.e. has not gone through Init/Setup/Boot).
func (v *VMM) runRestoredVM() error {
	g := new(errgroup.Group)

	for cpu := 0; cpu < v.NCPUs; cpu++ {
		i := cpu

		g.Go(func() error {
			return v.VCPU(os.Stderr, i, v.TraceCount)
		})
	}

	if err := g.Wait(); err != nil {
		log.Print(err)
	}

	return nil
}

// buildSnapshot collects the full VM snapshot from a running VMM.
func buildSnapshot(v *VMM) (*migration.Snapshot, error) {
	snap := &migration.Snapshot{
		NCPUs:   v.NCPUs,
		MemSize: v.MemSize,
	}

	// Per-vCPU state.
	snap.VCPUStates = make([]migration.VCPUState, v.NCPUs)

	for i := 0; i < v.NCPUs; i++ {
		s, err := v.SaveCPUState(i)
		if err != nil {
			return nil, fmt.Errorf("SaveCPUState %d: %w", i, err)
		}

		snap.VCPUStates[i] = *s
	}

	// VM-level state.
	vmState, err := v.SaveVMState()
	if err != nil {
		return nil, fmt.Errorf("SaveVMState: %w", err)
	}

	snap.VM = *vmState

	// Device state.
	ds, err := v.SaveDeviceState()
	if err != nil {
		return nil, fmt.Errorf("SaveDeviceState: %w", err)
	}

	snap.Devices = *ds

	return snap, nil
}

// applySnapshot restores a Snapshot onto machine m.
func applySnapshot(m *machine.Machine, snap *migration.Snapshot) error {
	for i := range snap.VCPUStates {
		if err := m.RestoreCPUState(i, &snap.VCPUStates[i]); err != nil {
			return err
		}
	}

	if err := m.RestoreVMState(&snap.VM); err != nil {
		return err
	}

	if err := m.RestoreDeviceState(&snap.Devices); err != nil {
		return err
	}

	return nil
}

// collectDirtyPages gathers dirty page data from the machine using bitmap
// and returns the raw bitmap bytes and packed page data.
func collectDirtyPages(m *machine.Machine, bitmap []uint64) (bitmapBytes []byte, pageData []byte, err error) {
	// Encode bitmap as little-endian uint64 words.
	bitmapBytes = make([]byte, len(bitmap)*8)
	for i, w := range bitmap {
		binary.LittleEndian.PutUint64(bitmapBytes[i*8:], w)
	}

	// Collect the actual page bytes.
	var buf bytes.Buffer

	if _, err := m.TransferDirtyPages(&buf, bitmap); err != nil {
		return nil, nil, err
	}

	return bitmapBytes, buf.Bytes(), nil
}

// applyDirtyPages restores dirty pages from bitmapBytes + pageData onto m.
func applyDirtyPages(m *machine.Machine, bitmapBytes []byte, pageData []byte) error {
	const pageSize = 4096

	if len(bitmapBytes)%8 != 0 {
		return fmt.Errorf("%w: %d", errBitmapLengthNotMult8, len(bitmapBytes))
	}

	mem := m.Mem()
	pageIdx := 0
	offset := 0

	for wi := 0; wi < len(bitmapBytes); wi += 8 {
		word := binary.LittleEndian.Uint64(bitmapBytes[wi:])

		for bit := 0; bit < 64; bit++ {
			pageBase := (wi/8*64 + bit) * pageSize

			if word&(1<<uint(bit)) != 0 {
				if offset+pageSize > len(pageData) {
					return fmt.Errorf("%w: at page %d", errPageDataTruncated, pageIdx)
				}

				if pageBase+pageSize <= len(mem) {
					copy(mem[pageBase:], pageData[offset:offset+pageSize])
				}

				offset += pageSize
				pageIdx++
			}
		}
	}

	return nil
}

// popcount counts the number of set bits in a uint64.
func popcount(x uint64) int {
	n := 0

	for x != 0 {
		n += int(x & 1)
		x >>= 1
	}

	return n
}
