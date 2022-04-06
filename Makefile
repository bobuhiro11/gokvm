GOLANGCI_LINT_VERSION = v1.35.2
BUSYBOX_VERSION = 1.33.1
LINUX_VERSION = 5.14.3
PCIUTILS_VERSION = 3.7.0
ETHTOOL_VERSION = 5.15
NUMCPUS=`grep -c '^processor' /proc/cpuinfo`
GUEST_IPV4_ADDR = 192.168.20.1/24

gokvm: $(wildcard *.go)
	go build .

golangci-lint:
	curl --retry 5 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b . $(GOLANGCI_LINT_VERSION)

busybox.tar.bz2:
	curl --retry 5 https://busybox.net/downloads/busybox-$(BUSYBOX_VERSION).tar.bz2 -o busybox.tar.bz2

pciutils.tar.gz:
	curl --retry 5 https://mirrors.edge.kernel.org/pub/software/utils/pciutils/pciutils-$(PCIUTILS_VERSION).tar.gz \
		-o pciutils.tar.gz

ethtool.tar.gz:
	curl --retry 5 http://ftp.ntu.edu.tw/pub/software/network/ethtool/ethtool-$(ETHTOOL_VERSION).tar.gz \
		-o ethtool.tar.gz

initrd: busybox.config busybox.tar.bz2 busybox.inittab busybox.passwd busybox.rcS pciutils.tar.gz ethtool.tar.gz
	[ -f _pciutils/README ] || tar -xf pciutils.tar.gz --one-top-level=_pciutils --strip-components 1
	$(MAKE) -C _pciutils \
		OPT="-O2 -static -static-libstdc++ -static-libgcc" \
		LDFLAGS=-static ZLIB=no DNS=no LIBKMOD=no HWDB=no
	[ -f _ethtool/README ] || tar -xf ethtool.tar.gz --one-top-level=_ethtool --strip-components 1
	cd _ethtool && ./autogen.sh && ./configure LDFLAGS=-static && $(MAKE)
	[ -f _busybox/README ] || tar -xf busybox.tar.bz2 --one-top-level=_busybox --strip-components 1
	cp busybox.config _busybox/.config
	$(MAKE) -C _busybox install
	mkdir -p _busybox/_install/usr/local/share
	mkdir -p _busybox/_install/etc/init.d
	mkdir -p _busybox/_install/proc
	mkdir -p _busybox/_install/sys
	rm -f _busybox/_install/usr/bin/lspci
	cp _pciutils/lspci _busybox/_install/usr/bin/lspci
	cp _pciutils/pci.ids _busybox/_install/usr/local/share/pci.ids
	cp _ethtool/ethtool _busybox/_install/usr/bin/ethtool
	cp busybox.inittab _busybox/_install/etc/inittab
	cp busybox.passwd  _busybox/_install/etc/passwd
	cp busybox.rcS     _busybox/_install/etc/init.d/rcS
	sed -i -e 's|{{ GUEST_IPV4_ADDR }}|$(GUEST_IPV4_ADDR)|g' _busybox/_install/etc/init.d/rcS
	cd _busybox/_install && find . | cpio -o --format=newc > ../../initrd

linux.tar.xz:
	curl --retry 5 https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-$(LINUX_VERSION).tar.xz \
		-o linux.tar.xz

bzImage: linux.config linux.tar.xz
	[ -f _linux/README ] || tar Jxf ./linux.tar.xz --one-top-level=_linux --strip-components 1
	cp linux.config _linux/.config
	$(MAKE) -C _linux
	cp _linux/arch/x86/boot/bzImage .

.PHONY: run
run: initrd bzImage
	go run . -c 4

.PHONY: run-system-kernel
run-system-kernel:
	# Implemented based on fedora's default path.
	# Other distributions need to be considered.
	go run . -p "console=ttyS0 pci=off earlyprintk=serial nokaslr rdinit=/bin/sh" \
		-k $(shell ls -t /boot/vmlinuz*.x86_64 | head -n 1) \
		-i $(shell ls -t /boot/initramfs*.x86_64.img | head -n 1)

# N.B. the golangci-lint recommends you not enable --enable-all (which begs the
# question of why it's there in the first place) as upgrades to golangci-lint
# can break your CI!
.PHONY: golangci
golangci: golangci-lint
	./golangci-lint run --enable-all \
		--disable gomnd \
		--disable wrapcheck \
		--disable maligned \
		--disable forbidigo \
		--disable funlen \
		--disable gocognit \
		./...

test: golangci initrd bzImage
	go test -coverprofile c.out ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint ._busybox initrd bzImage _linux \
		_ethtool ethtool.tar.gz _pciutils pciutils.tar.gz

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/bin/init" --enable-kvm
