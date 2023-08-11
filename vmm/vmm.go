package vmm

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/bobuhiro11/gokvm/flag"
	"github.com/bobuhiro11/gokvm/kvm"
	"github.com/bobuhiro11/gokvm/machine"
	"github.com/bobuhiro11/gokvm/term"
)

type VMM struct {
	*machine.Machine
	flag.Config
}

func New(c flag.Config) *VMM {
	return &VMM{
		Machine: nil,
		Config:  c,
	}
}

// Init instantiates a machine.
func (v *VMM) Init() error {
	m, err := machine.New(v.Dev, v.NCPUs, v.MemSize)
	if err != nil {
		return err
	}

	if len(v.TapIfName) > 0 {
		if err := m.AddTapIf(v.TapIfName); err != nil {
			return err
		}
	}

	if len(v.Disk) > 0 {
		if err := m.AddDisk(v.Disk); err != nil {
			return err
		}
	}

	v.Machine = m

	return nil
}

func (v *VMM) Setup() error {
	kern, err := os.Open(v.Kernel)
	if err != nil {
		return err
	}

	initrd, err := os.Open(v.Initrd)
	if err != nil {
		return err
	}

	if err := v.Machine.LoadLinux(kern, initrd, v.Params); err != nil {
		return err
	}

	return nil
}

func (v *VMM) Boot() error {
	var err error

	var wg sync.WaitGroup

	trace := v.TraceCount > 0
	if err := v.SingleStep(trace); err != nil {
		return fmt.Errorf("setting trace to %v:%w", trace, err)
	}

	for cpu := 0; cpu < v.NCPUs; cpu++ {
		fmt.Printf("Start CPU %d of %d\r\n", cpu, v.NCPUs)
		wg.Add(1)

		go func(cpu int) {
			// Consider ANOTHER option, maxInsCount, which would
			// exit this loop after a certain number of instructions
			// were run.
			for tc := 0; ; tc++ {
				err = v.RunInfiniteLoop(cpu)
				if err == nil {
					continue
				}

				if !errors.Is(err, kvm.ErrDebug) {
					break
				}

				if err := v.SingleStep(trace); err != nil {
					fmt.Printf("Setting trace to %v:%v", trace, err)
				}

				if tc%v.TraceCount != 0 {
					continue
				}

				_, r, s, err := v.Inst(cpu)
				if err != nil {
					fmt.Printf("disassembling after debug exit:%v", err)
				} else {
					fmt.Printf("%#x:%s\r\n", r.RIP, s)
				}
			}

			wg.Done()
			fmt.Printf("CPU %d exits\n\r", cpu)
		}(cpu)
	}

	if !term.IsTerminal() {
		fmt.Fprintln(os.Stderr, "this is not terminal and does not accept input")
		select {}
	}

	restoreMode, err := term.SetRawMode()
	if err != nil {
		return err
	}

	defer restoreMode()

	var before byte = 0

	in := bufio.NewReader(os.Stdin)

	if err := v.SingleStep(trace); err != nil {
		log.Printf("SingleStep(%v): %v", trace, err)

		return err
	}

	go func() {
		for {
			b, err := in.ReadByte()
			if err != nil {
				log.Printf("%v", err)

				break
			}
			v.GetInputChan() <- b

			if len(v.GetInputChan()) > 0 {
				if err := v.InjectSerialIRQ(); err != nil {
					log.Printf("InjectSerialIRQ: %v", err)
				}
			}

			if before == 0x1 && b == 'x' {
				restoreMode()
				os.Exit(0)
			}

			before = b
		}
	}()

	fmt.Printf("Waiting for CPUs to exit\r\n")
	wg.Wait()
	fmt.Printf("All cpus done\n\r")

	return nil
}
