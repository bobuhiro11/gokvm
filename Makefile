GOLANGCI_LINT_VERSION = v1.35.2
BUSYBOX_VERSION = 1.33.0
LINUX_VERSION = 5.10.12

gokvm: $(wildcard *.go)
	go build .

golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b . $(GOLANGCI_LINT_VERSION)

initrd:
	curl https://busybox.net/downloads/busybox-$(BUSYBOX_VERSION).tar.bz2 -o busybox.tar.bz2
	tar -xf busybox.tar.bz2
	cp busybox.config busybox-$(BUSYBOX_VERSION)/.config
	make -C busybox-$(BUSYBOX_VERSION) install
	mkdir -p busybox-$(BUSYBOX_VERSION)/_install/etc
	cp inittab busybox-$(BUSYBOX_VERSION)/_install/etc/inittab
	cd busybox-$(BUSYBOX_VERSION)/_install && find . | cpio -o --format=newc > ../../initrd
	rm -rf busybox-$(BUSYBOX_VERSION)

bzImage:
	curl https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-5.10.12.tar.xz \
		-o linux-$(LINUX_VERSION).tar.xz
	tar Jxf ./linux-5.10.12.tar.xz
	cp linux.config linux-$(LINUX_VERSION)/.config
	make -C linux-$(LINUX_VERSION)
	cp linux-$(LINUX_VERSION)/arch/x86/boot/bzImage .
	rm -rf linux-$(LINUX_VERSION)

.PHONY: run
run:
	go run .

.PHONY: test
test: golangci-lint initrd bzImage
	./golangci-lint run --enable-all --disable gomnd --disable wrapcheck ./...
	ls -la /dev/kvm
	go test -v ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint .busybox-$(BUSYBOX_VERSION) initrd bzImage linux-$(LINUX_VERSION)

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/bin/init" --enable-kvm
