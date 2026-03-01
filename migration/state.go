// Package migration provides types and utilities for live migration of gokvm VMs.
package migration

// MSREntry is an index/value pair for a model-specific register.
type MSREntry struct {
	Index uint32
	Data  uint64
}

// VCPUState holds the complete architectural state of a single vCPU.
// Binary KVM structs are stored as raw byte slices to preserve their exact
// in-memory layout (including padding) without encoding ambiguity.
type VCPUState struct {
	Regs      []byte     // kvm.Regs
	Sregs     []byte     // kvm.Sregs
	MSRs      []MSREntry // model-specific registers
	LAPIC     []byte     // kvm.LAPICState
	Events    []byte     // kvm.VCPUEvents
	MPState   uint32     // kvm.MPState.State
	DebugRegs []byte     // kvm.DebugRegs
	XCRS      []byte     // kvm.XCRS
}

// VMState holds VM-level (not per-vCPU) hardware state.
type VMState struct {
	Clock         []byte // kvm.ClockData
	IRQChipPIC0   []byte // kvm.IRQChip ChipID=0 (master PIC)
	IRQChipPIC1   []byte // kvm.IRQChip ChipID=1 (slave PIC)
	IRQChipIOAPIC []byte // kvm.IRQChip ChipID=2 (IOAPIC)
	PIT2          []byte // kvm.PITState2
}

// BlkState holds migration state for a virtio-blk device.
type BlkState struct {
	// HdrBytes is the serialized blkHdr (virtio common header + blk config),
	// encoded with binary.LittleEndian to preserve all fields including padding.
	HdrBytes      []byte
	QueuePhysAddr [1]uint64 // guest physical address of each virtqueue (0 = not initialised)
	LastAvailIdx  [1]uint16 // host-side consumed index per queue
}

// NetState holds migration state for a virtio-net device.
type NetState struct {
	HdrBytes      []byte
	QueuePhysAddr [2]uint64
	LastAvailIdx  [2]uint16
}

// SerialState holds migration state for the emulated serial port.
type SerialState struct {
	IER byte // Interrupt Enable Register
	LCR byte // Line Control Register
}

// DeviceState aggregates emulated device state.
// Blk and Net are nil when the corresponding device is not attached.
type DeviceState struct {
	Serial SerialState
	Blk    *BlkState // nil if no disk attached
	Net    *NetState // nil if no network attached
}

// Snapshot is the complete VM state handed off during migration.
// Guest memory is transferred separately as a raw byte stream.
type Snapshot struct {
	NCPUs      int
	MemSize    int
	VCPUStates []VCPUState
	VM         VMState
	Devices    DeviceState
}
