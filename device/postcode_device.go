package device

import "fmt"

type PostCodeDevice struct{}

func (p *PostCodeDevice) Read(port uint64, data []byte) error {
	return nil
}

func (p *PostCodeDevice) Write(port uint64, data []byte) error {
	if len(data) != 1 {
		return errDataLenInvalid
	}

	if data[0] == '\000' {
		fmt.Printf("\r\n")
	} else {
		fmt.Printf("%c", data[0])
	}

	return nil
}

func (p *PostCodeDevice) IOPort() uint64 {
	return 0x80
}

func (p *PostCodeDevice) Size() uint64 {
	return 0x1
}
