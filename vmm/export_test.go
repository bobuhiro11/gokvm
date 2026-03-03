package vmm

// export_test.go exposes unexported vmm symbols for use by the external
// vmm_test package.  This file is compiled only during testing.

var (
	ControlSocketPath = controlSocketPath //nolint:gochecknoglobals
	ApplyDirtyPages   = applyDirtyPages   //nolint:gochecknoglobals
)
