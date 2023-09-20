package iodev

type NoopDevice struct {
	Port  uint64
	Psize uint64
}

func (n *NoopDevice) Read(port uint64, data []byte) error {
	return nil
}

func (n *NoopDevice) Write(port uint64, data []byte) error {
	return nil
}

func (n *NoopDevice) IOPort() uint64 {
	return n.Port
}

func (n *NoopDevice) Size() uint64 {
	return n.Psize
}
