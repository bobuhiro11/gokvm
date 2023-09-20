package iodev

import "log"

// This device is used by EDK2/CloudHv to let the host know about a shutdown.
// No implementation of handling the event on host side yet.
// See: https://github.com/cloud-hypervisor/edk2/blob/ch/OvmfPkg/Include/IndustryStandard/CloudHv.h

const (
	ACPIShutDownDevPort = uint64(0x600)
)

type ACPIShutDownDevice struct {
	Port uint64
	// ExitEvent  chan int
	// ResetEvent chan int
}

func NewACPIShutDownEvent() *ACPIShutDownDevice {
	return &ACPIShutDownDevice{
		Port: ACPIShutDownDevPort,
	}
}

func (a *ACPIShutDownDevice) Read(base uint64, data []byte) error {
	data[0] = 0

	return nil
}

func (a *ACPIShutDownDevice) Write(base uint64, data []byte) error {
	if data[0] == 1 {
		// Send 1 to ResetEvent
		// a.ResetEvent <- 1
		log.Println("ACPI Reboot signaled")
	}
	// The ACPI DSDT table specifies the S5 sleep state (shutdown) as value 5
	S5SleepVal := uint8(5)
	SleepStatusENBit := uint8(5)
	SleepValBit := uint8(2)

	if data[0] == (S5SleepVal<<SleepValBit)|(1<<SleepStatusENBit) {
		// a.ExitEvent <- 1
		log.Println("ACPI Shutdown signalled")
	}

	return nil
}

func (a *ACPIShutDownDevice) IOPort() uint64 {
	return a.Port
}

func (a *ACPIShutDownDevice) Size() uint64 {
	return 0x8
}
