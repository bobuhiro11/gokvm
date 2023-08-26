package flag

import (
	"log"

	"github.com/alecthomas/kong"
	"github.com/bobuhiro11/gokvm/probe"
	"github.com/bobuhiro11/gokvm/vmm"
)

func Parse() error {
	c := CLI{}

	programName := "gokvm"
	programDesc := "gokvm is a small Linux KVM Hypervisor which supports kernel boot"

	ctx := kong.Parse(&c,
		kong.Name(programName),
		kong.Description(programDesc),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}))

	err := ctx.Run()

	return err
}

func (d *ProbeCMD) Run() error {
	if err := probe.KVMCapabilities(); err != nil {
		return err
	}

	return nil
}

func (s *BootCMD) Run() error {
	defparams := `console=ttyS0 earlyprintk=serial noapic noacpi notsc ` +
		`debug apic=debug show_lapic=all mitigations=off lapic tsc_early_khz=2000 ` +
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" pci=realloc=off ` +
		`virtio_pci.force_legacy=1 rdinit=/init init=/init ` +
		`gokvm.ipv4_addr=192.168.20.1/24`

	memSize, err := ParseSize(s.MemSize, "g")
	if err != nil {
		return err
	}

	traceC, err := ParseSize(s.TraceCount, "")
	if err != nil {
		return err
	}

	if len(s.Params) > 0 {
		defparams = s.Params
	}

	c := &vmm.Config{
		Dev:        s.Dev,
		Kernel:     s.Kernel,
		Initrd:     s.Initrd,
		Params:     defparams,
		TapIfName:  s.TapIfName,
		Disk:       s.Disk,
		NCPUs:      s.NCPUs,
		MemSize:    memSize,
		TraceCount: traceC,
	}

	vmm := vmm.New(*c)

	if err := vmm.Init(); err != nil {
		log.Fatal(err)
	}

	if err := vmm.Setup(); err != nil {
		log.Fatal(err)
	}

	if err := vmm.Boot(); err != nil {
		log.Fatal(err)
	}

	return nil
}
