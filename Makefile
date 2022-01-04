GOLANGCI_LINT_VERSION = v1.35.2
BUSYBOX_VERSION = 1.33.1
LINUX_VERSION = 5.14.3
PCIUTILS_VERSION = 3.7.0
NUMCPUS=`grep -c '^processor' /proc/cpuinfo`

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

initrd: busybox.config busybox.tar.bz2 busybox.inittab busybox.passwd busybox.rcS pciutils.tar.gz
	tar -xf pciutils.tar.gz
	make -C pciutils-$(PCIUTILS_VERSION) \
		OPT="-O2 -static -static-libstdc++ -static-libgcc" \
		LDFLAGS=-static ZLIB=no DNS=no LIBKMOD=no HWDB=no
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
	cp busybox.inittab busybox-$(BUSYBOX_VERSION)/_install/etc/inittab
	cp busybox.passwd  busybox-$(BUSYBOX_VERSION)/_install/etc/passwd
	cp busybox.rcS     busybox-$(BUSYBOX_VERSION)/_install/etc/init.d/rcS
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
	./golangci-lint run --enable-all \
		--disable gomnd \
		--disable wrapcheck \
		--disable maligned \
		--disable forbidigo \
		--disable funlen \
		$(shell find . -type f -name "*.go" | xargs dirname | sort)
	go test -v -coverprofile c.out $(shell find . -type f -name "*.go" | xargs dirname | sort)

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint .busybox-$(BUSYBOX_VERSION) initrd bzImage linux-$(LINUX_VERSION)

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/bin/init" --enable-kvm
