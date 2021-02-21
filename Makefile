GOLANGCI_LINT_VERSION = v1.35.2
BUSYBOX_VERSION = 1.33.0
LINUX_VERSION = 5.10.12

gokvm: $(wildcard *.go)
	go build .

golangci-lint:
	curl --retry 5 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b . $(GOLANGCI_LINT_VERSION)

busybox.tar.bz2:
	curl --retry 5 https://busybox.net/downloads/busybox-$(BUSYBOX_VERSION).tar.bz2 -o busybox.tar.bz2

initrd: busybox.config busybox.tar.bz2 busybox.inittab busybox.passwd busybox.rcS
	tar -xf busybox.tar.bz2
	cp busybox.config busybox-$(BUSYBOX_VERSION)/.config
	make -C busybox-$(BUSYBOX_VERSION) install
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/etc/init.d
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/proc
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/sys
	cp busybox.inittab busybox-$(BUSYBOX_VERSION)/_install/etc/inittab
	cp busybox.passwd  busybox-$(BUSYBOX_VERSION)/_install/etc/passwd
	cp busybox.rcS     busybox-$(BUSYBOX_VERSION)/_install/etc/init.d/rcS
	cd busybox-$(BUSYBOX_VERSION)/_install && find . | cpio -o --format=newc > ../../initrd
	rm -rf busybox-$(BUSYBOX_VERSION)

linux.tar.xz:
	curl --retry 5 https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-$(LINUX_VERSION).tar.xz \
		-o linux.tar.xz

bzImage: linux.config linux.tar.xz
	tar Jxf ./linux.tar.xz
	cp linux.config linux-$(LINUX_VERSION)/.config
	make -C linux-$(LINUX_VERSION)
	cp linux-$(LINUX_VERSION)/arch/x86/boot/bzImage .
	rm -rf linux-$(LINUX_VERSION)

.PHONY: run
run: initrd bzImage
	go run .

.PHONY: test
test: golangci-lint initrd bzImage
	./golangci-lint run --enable-all \
		--disable gomnd \
		--disable wrapcheck \
		--disable maligned \
		--disable forbidigo \
		--disable funlen \
		./...
	go test -v -coverprofile c.out ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint .busybox-$(BUSYBOX_VERSION) initrd bzImage linux-$(LINUX_VERSION)

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/bin/init" --enable-kvm
