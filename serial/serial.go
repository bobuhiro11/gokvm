package serial

import (
	"fmt"
)

const (
	COM1Addr = 0x03f8
)

type Serial struct {
	IER byte
	LCR byte

	inputChan chan byte

	// This callback is called when serial request IRQ.
	irqCallback func(irq, level uint32)
}

func New(irqCallBack func(irq, level uint32)) (*Serial, error) {
	s := &Serial{
		IER: 0, LCR: 0,
		inputChan:   make(chan byte, 10000),
		irqCallback: irqCallBack,
	}

	return s, nil
}

func (s *Serial) GetInputChan() chan<- byte {
	return s.inputChan
}

func (s *Serial) dlab() bool {
	return s.LCR&0x80 != 0
}

func (s *Serial) InjectIRQ(level uint32) {
	s.irqCallback(4, level)
}

func (s *Serial) In(port uint64, values []byte) error {
	port -= COM1Addr

	switch {
	case port == 0 && !s.dlab():
		// RBR
		if len(s.inputChan) > 0 {
			values[0] = <-s.inputChan
		}
	case port == 0 && s.dlab():
		// DLL
		values[0] = 0xc // baud rate 9600
		fmt.Printf("[IN  DLL] value: %#v\n", values)
	case port == 1 && !s.dlab():
		// IER
		values[0] = s.IER
		// fmt.Printf("[IN  IER] value: %#v\n", values)
	case port == 1 && s.dlab():
		// DLM
		values[0] = 0x0 // baud rate 9600
		fmt.Printf("[IN  DLM] value: %#v\n", values)
	case port == 2:
		// IIR
		// fmt.Printf("[IN  IIR] value: %#v\n", values)
	case port == 3:
		// LCR
		fmt.Printf("[IN  LCR] value: %#v\n", values)
	case port == 4:
		// MCR
		fmt.Printf("[IN  MCR] value: %#v\n", values)
	case port == 5:
		// LSR
		values[0] = 0x60 // THR is empty
		if len(s.inputChan) > 0 {
			values[0] |= 0x1 // Data available
		}
		// fmt.Printf("[IN  LSR] value: %#v\n", values)
	case port == 6:
		// MSR
		// fmt.Printf("[IN  MSR] value: %#v\n", values)
		break
	}

	return nil
}

func (s *Serial) Out(port uint64, values []byte) error {
	port -= COM1Addr

	switch {
	case port == 0 && !s.dlab():
		// THR
		fmt.Printf("%c", values[0])
	case port == 0 && s.dlab():
		// DLL
		fmt.Printf("[OUT DLL] value: %#v\n", values)
	case port == 1 && !s.dlab():
		// IER
		s.IER = values[0]
		if s.IER != 0 {
			s.InjectIRQ(0)
			s.InjectIRQ(1)
		}
		// fmt.Printf("[OUT IER] value: %#v\n", values)
	case port == 1 && s.dlab():
		// DLM
		fmt.Printf("[OUT DLM] value: %#v\n", values)
	case port == 2:
		// FCR
		fmt.Printf("[OUT FCR] value: %#v\n", values)
	case port == 3:
		// LCR
		s.LCR = values[0]
		fmt.Printf("[OUT LCR] value: %#v\n", values)
	case port == 4:
		// MCR
		fmt.Printf("[OUT MCR] value: %#v\n", values)
	default:
		fmt.Printf("factory test or not used\n")
	}

	return nil
}
