GOLANGCI_LINT_VERSION = v1.35.2
BUSYBOX_VERSION = 1.33.1
LINUX_VERSION = 5.14.3
PCIUTILS_VERSION = 3.7.0
ETHTOOL_VERSION = 5.15
NUMCPUS=`grep -c '^processor' /proc/cpuinfo`
GUEST_IPV4_ADDR = 192.168.10.1/24
HOST_IPV4_ADDR = 192.168.10.2/24

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
	tar -xf pciutils.tar.gz
	make -C pciutils-$(PCIUTILS_VERSION) \
		OPT="-O2 -static -static-libstdc++ -static-libgcc" \
		LDFLAGS=-static ZLIB=no DNS=no LIBKMOD=no HWDB=no
	tar -xf ethtool.tar.gz
	cd ./ethtool-$(ETHTOOL_VERSION) && ./autogen.sh && ./configure LDFLAGS=-static && make
	tar -xf busybox.tar.bz2
	cp busybox.config busybox-$(BUSYBOX_VERSION)/.config
	$(MAKE) -C busybox-$(BUSYBOX_VERSION) install
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/usr/local/share
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/etc/init.d
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/proc
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/sys
	rm -f busybox-$(BUSYBOX_VERSION)/_install/usr/bin/lspci
	cp pciutils-$(PCIUTILS_VERSION)/lspci busybox-$(BUSYBOX_VERSION)/_install/usr/bin/lspci
	cp pciutils-$(PCIUTILS_VERSION)/pci.ids busybox-$(BUSYBOX_VERSION)/_install/usr/local/share/pci.ids
	cp ethtool-$(ETHTOOL_VERSION)/ethtool busybox-$(BUSYBOX_VERSION)/_install/usr/bin/ethtool
	cp busybox.inittab busybox-$(BUSYBOX_VERSION)/_install/etc/inittab
	cp busybox.passwd  busybox-$(BUSYBOX_VERSION)/_install/etc/passwd
	cp busybox.rcS     busybox-$(BUSYBOX_VERSION)/_install/etc/init.d/rcS
	sed -i -e 's|{{ GUEST_IPV4_ADDR }}|$(GUEST_IPV4_ADDR)|g' busybox-$(BUSYBOX_VERSION)/_install/etc/init.d/rcS
	cd busybox-$(BUSYBOX_VERSION)/_install && find . | cpio -o --format=newc > ../../initrd
	rm -rf busybox-$(BUSYBOX_VERSION)

linux.tar.xz:
	curl --retry 5 https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-$(LINUX_VERSION).tar.xz \
		-o linux.tar.xz

bzImage: linux.config linux.tar.xz
	if ! test -f linux-$(LINUX_VERSION)/.config; then \
		tar Jxf ./linux.tar.xz; \
		cp linux.config linux-$(LINUX_VERSION)/.config; \
	fi
	$(MAKE) -C linux-$(LINUX_VERSION)
	cp linux-$(LINUX_VERSION)/arch/x86/boot/bzImage .

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

.PHONY: test
test: golangci-lint initrd bzImage
	-pkill -f gokvm
	./golangci-lint run --enable-all \
		--disable gomnd \
		--disable wrapcheck \
		--disable maligned \
		--disable forbidigo \
		--disable funlen \
		$(shell find . -type f -name "*.go" | xargs dirname | sort)
	go test -v -coverprofile c.out $(shell find . -type f -name "*.go" | xargs dirname | sort)
	# launch the executable & check ping
	$(MAKE) run > output.log 2>&1 &
	sleep 1s && ip link set tap up && ip addr add $(HOST_IPV4_ADDR) dev tap
	sleep 5s && cat output.log
	ping $(shell echo $(GUEST_IPV4_ADDR) | sed -e 's|/.*$$||g') -c 3
	-pkill -f gokvm

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint .busybox-$(BUSYBOX_VERSION) initrd bzImage linux-$(LINUX_VERSION)

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/bin/init" --enable-kvm
