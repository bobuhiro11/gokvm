# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when
working with code in this repository.

## What is gokvm

A lightweight x86-64 hypervisor written in Go using Linux KVM.
Boots unmodified Linux kernels (5.10+) with serial console,
virtio-net, and virtio-blk device emulation in ~1.5k lines.

## Build & Test Commands

```bash
# Build (runs go generate then go build)
make gokvm

# Run full test suite (builds images, lints, tests with coverage)
# Requires sudo for KVM access
sudo make test

# Run a single test
sudo go test -v -run TestName ./package

# Lint (golangci-lint, enable-all with selected exclusions)
make golangci

# Generate code (stringer for enum types)
make generate

# Build test artifacts individually
make bzImage vmlinux initrd vda.img
```

Tests require system packages: qemu-kvm, qemu-utils, libmnl-dev,
genext2fs. Tests need KVM access and run under sudo.

## Architecture

### Entry Point & Boot Flow

`main.go` dispatches to `boot` or `probe` subcommands.

Boot flow: `VMM.Init()` → `VMM.Setup()` → `VMM.Boot()`
- **Init**: Opens /dev/kvm, creates VM, allocates guest RAM,
  sets up IRQ chip, PIT, registers I/O port handlers
- **Setup**: Detects boot protocol (bzImage or PVH), loads
  kernel/initrd into guest memory, configures page tables
- **Boot**: Spawns one goroutine per vCPU, each in a
  Run→ExitReason→dispatch loop

### Key Packages

| Package | Role |
|---------|------|
| vmm | Orchestrator: Config, Init, Setup, Boot |
| machine | Core VM state, memory, vCPU loop, I/O dispatch |
| kvm | Thin ioctl wrapper for /dev/kvm |
| virtio | virtio-net and virtio-blk devices |
| pci | PCI config space emulation (ports 0xcf8/0xcfc) |
| serial | COM1 serial console (port 0x3f8) |
| iodev | Simple devices: NOOP, PostCode, CMOS, ACPI PM Timer |
| pvh | PVH boot protocol support |
| bootparam | bzImage boot parameter parsing |
| ebda | EBDA/MP table structures |

### Core Interfaces

**I/O Device** (`iodev.Device`): Read/Write/IOPort/Size — used
for non-PCI devices (serial, CMOS, etc.)

**PCI Device** (`pci.Device`): extends iodev with
`GetDeviceHeader()` — used for virtio-net and virtio-blk.

### I/O Dispatch

`machine.Machine` has a 64K×2 handler table
(`ioportHandlers[port][direction]`). On vCPU EXITIO, the port
and direction select the handler. Devices register ranges at
init time via `registerIOPortHandler`.

### Virtio Devices

virtio-net and virtio-blk each run async goroutines
(TxThreadEntry, RxThreadEntry, IOThreadEntry) that process
VirtQueue descriptors independently from vCPU execution.

## CI

GitHub Actions on ubuntu-22.04 with Go 1.21.x and 1.22.x.
Triggered on push/PR to main and daily. 60-minute timeout.
