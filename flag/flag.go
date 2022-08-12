package flag

import (
	"flag"
)

// ParseArgs calls flag.Parse and returns strings for device, kernel, initrd, bootparams, tapIfName, disk, and nCpus.
func ParseArgs(args []string) (string, string, string, string, string, string, int, error) {
	kernel := flag.String("k", "./bzImage", "kernel image path")
	initrd := flag.String("i", "./initrd", "initrd path")
	nCpus := flag.Int("c", 1, "number of cpus")
	tapIfName := flag.String("t", "tap", "name of tap interface")
	disk := flag.String("d", "/dev/zero", "path of disk file (for /dev/vda)")
	kvmPath := flag.String("D", "/dev/kvm", "path of kvm device")

	//  refs: commit 1621292e73770aabbc146e72036de5e26f901e86 in kvmtool
	params := flag.String("p", `console=ttyS0 earlyprintk=serial noapic noacpi notsc `+
		`debug apic=debug show_lapic=all mitigations=off lapic tsc_early_khz=2000 `+
		`dyndbg="file arch/x86/kernel/smpboot.c +plf ; file drivers/net/virtio_net.c +plf" pci=realloc=off `+
		`virtio_pci.force_legacy=1 rdinit=/init init=/init`, "kernel command-line parameters")

	flag.Parse()

	if err := flag.CommandLine.Parse(args[1:]); err != nil {
		return "", "", "", "", "", "", 0, err
	}

	return *kvmPath, *kernel, *initrd, *params, *tapIfName, *disk, *nCpus, nil
}
