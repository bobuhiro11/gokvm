GOLANGCI_LINT_VERSION = v1.46.0
NUMCPUS=`grep -c '^processor' /proc/cpuinfo`
GUEST_IPV4_ADDR = 192.168.20.1/24

gokvm: $(wildcard *.go) $(wildcard */*.go)
	$(MAKE) generate
	go build .

golangci-lint:
	curl --retry 5 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b . $(GOLANGCI_LINT_VERSION)

vda.img:
	$(eval dir = $(shell mktemp -d))
	echo "index.html: this message is from /dev/vda in guest" > ${dir}/index.html
	genext2fs -b 1024 -d ${dir} $@
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

bzImage: linux.config
	./scripts/get_kernel.bash

vmlinux: linux.config
	./scripts/get_kernel.bash

.PHONY: run
run: initrd bzImage
	$(MAKE) generate
	go run . -c 4

.PHONY: run-system-kernel
run-system-kernel:
	$(MAKE) generate
	# Implemented based on fedora's default path.
	# Other distributions need to be considered.
	go run . -p "console=ttyS0 pci=off earlyprintk=serial nokaslr rdinit=/bin/sh" \
		-k $(shell ls -t /boot/vmlinuz*.x86_64 | head -n 1) \
		-i $(shell ls -t /boot/initramfs*.x86_64.img | head -n 1)

.PHONY: generate
generate:
	go generate ./...

.PHONY: golangci
golangci: golangci-lint
	$(MAKE) generate
	./golangci-lint run ./...

.PHONY: test
test: bzImage vda.img
	$(MAKE) generate
	$(MAKE) golangci
	go test -coverprofile c.out ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint bzImage vmlinux _linux *_string.go

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/init" --enable-kvm
