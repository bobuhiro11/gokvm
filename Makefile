GOLANGCI_LINT_VERSION = v1.35.2
LINUX_VERSION = 5.14.3
NUMCPUS=`grep -c '^processor' /proc/cpuinfo`
GUEST_IPV4_ADDR = 192.168.20.1/24

gokvm: $(wildcard *.go)
	go build .

golangci-lint:
	curl --retry 5 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b . $(GOLANGCI_LINT_VERSION)

vda.img:
	dd if=/dev/zero of=$@ bs=1024k count=10
	mkfs.ext2 $@
	mkdir -p mnt_test
	mount -o loop -t ext2 $@ mnt_test
	echo "index.html: this message is from /dev/vda in guest" > mnt_test/index.html
	ls -la mnt_test
	umount mnt_test
	file $@

# because a go-based VMM deserves a go initrd.
# but we include bash (because people like it) and other handy tools.
# we include the local host tools; the u-root -files command will arrange to bring
# in all the needed .so
# You need to have installed the u-root command.
# GOPATH needs to be set.
# Something weird here: if I use $SHELL in this it expands to /bin/sh *in this makefile*, but not outside. WTF?
initrd:
	sed -i -e 's|{{ GUEST_IPV4_ADDR }}|$(GUEST_IPV4_ADDR)|g' .bashrc
	(cd $(GOPATH)/src/github.com/u-root/u-root && \
			u-root \
			-defaultsh `which bash` \
			-o $(PWD)/initrd \
			-files `which ethtool` \
			-files `which lspci` \
			-files `which lsblk` \
			-files `which hexdump` \
			-files `which mount` \
			-files `which bash` \
			-files `which nohup` \
			-files `which clear` \
			-files `which tic` \
			-files "/usr/share/terminfo/l/linux-c:/usr/share/terminfo/l/linux" \
			-files "/usr/share/misc/pci.ids" \
			-files "$(PWD)/.bashrc:.bashrc" \
			core boot github.com/u-root/u-root/cmds/exp/srvfiles)

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

test: golangci initrd bzImage vda.img
	go test -coverprofile c.out ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint bzImage _linux

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/init" --enable-kvm
