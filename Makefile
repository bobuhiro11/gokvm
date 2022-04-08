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

vda.img:
	dd if=/dev/zero of=$@ bs=1024k count=10
	mkfs.ext2 $@
	mkdir -p mnt_test
	mount -o loop -t ext2 $@ mnt_test
	echo "index.html: this message is from /dev/vda in guest" > mnt_test/index.html
	ls -la mnt_test
	umount mnt_test
	file $@

initrd: busybox.config busybox.tar.bz2 busybox.inittab busybox.passwd busybox.rcS pciutils.tar.gz ethtool.tar.gz
	tar -xf pciutils.tar.gz --one-top-level=_pciutils --strip-components 1
	$(MAKE) -C _pciutils \
		OPT="-O2 -static -static-libstdc++ -static-libgcc" \
		LDFLAGS=-static ZLIB=no DNS=no LIBKMOD=no HWDB=no
	tar -xf ethtool.tar.gz --one-top-level=_ethtool --strip-components 1
	cd _ethtool && ./autogen.sh && ./configure LDFLAGS=-static && $(MAKE)
	tar -xf busybox.tar.bz2 --one-top-level=_busybox --strip-components 1
	cp busybox.config _busybox/.config
	$(MAKE) -C _busybox install
	mkdir -p _busybox/_install/usr/local/share
	mkdir -p _busybox/_install/etc/init.d
	mkdir -p _busybox/_install/proc
	mkdir -p _busybox/_install/sys
	mkdir -p _busybox/_install/dev
	mkdir -p _busybox/_install/mnt/dev_vda
	[ -e _busybox/_install/dev/null ] || mknod _busybox/_install/dev/null c   1 3
	[ -e _busybox/_install/dev/zero ] || mknod _busybox/_install/dev/zero c   1 5
	[ -e _busybox/_install/dev/vda  ] || mknod _busybox/_install/dev/vda  b 254 0
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
	if ! test -f _linux/.config; then \
		tar Jxf ./linux.tar.xz --one-top-level=_linux --strip-components 1; \
		cp linux.config _linux/.config; \
	fi
	$(MAKE) -C _linux
	cp _linux/arch/x86/boot/bzImage .

.PHONY: run
run: initrd bzImage vda.img
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

test: golangci initrd bzImage vda.img
	go test -coverprofile c.out ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint ._busybox initrd bzImage _linux \
		_ethtool ethtool.tar.gz _pciutils pciutils.tar.gz

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/bin/init" --enable-kvm
