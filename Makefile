GOLANGCI_LINT_VERSION = v1.54.2
NUMCPUS=`grep -c '^processor' /proc/cpuinfo`

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

# checkbinaries runs which on all the commands we want to include.
# Be sure to keep it up to date if you add new commands to the initrd
# rule below.
checkbinaries:
	@which ethtool
	@which lspci
	@which lsblk
	@which hexdump
	@which mount
	@which bash
	@which nohup
	@which clear
	@which tic
	@which awk
	@which grep
	@which cut

initrd: checkbinaries ./scripts/get_initrd.bash
	./scripts/get_initrd.bash

bzImage vmlinux: linux.config ./scripts/get_kernel.bash
	./scripts/get_kernel.bash

bzImage_PVH vmlinux_PVH CLOUDHV.fd: linux_pvh.config ./scripts/get_kernel.bash
	./scripts/get_kernel.bash \
		bzImage_PVH \
		vmlinux_PVH \
		linux_pvh.config

.PHONY: run
run: initrd bzImage
	$(MAKE) generate
	go run . boot -c 4 -i "./initrd"

.PHONY: runpvh
runpvh: initrd vmlinux_PVH
	$(MAKE) generate
	go run . boot -c 4 -k "./vmlinuz_PVH" -i "./initrd"

.PHONY: run-system-kernel
run-system-kernel:
	$(MAKE) generate
	# Implemented based on fedora's default path.
	# Other distributions need to be considered.
	go run . boot -k $(shell ls -t /boot/vmlinuz*.x86_64 | head -n 1) \
		-p "console=ttyS0 pci=off earlyprintk=serial nokaslr rdinit=/bin/sh" \
		-i $(shell ls -t /boot/initramfs*.x86_64.img | head -n 1)

.PHONY: generate
generate:
	go generate ./...

.PHONY: golangci
golangci: golangci-lint
	$(MAKE) generate
	./golangci-lint run ./...

.PHONY: test
test: bzImage vmlinux vmlinux_PVH initrd vda.img CLOUDHV.fd
	$(MAKE) generate
	$(MAKE) golangci
	go test -coverprofile c.out ./...

.PHONY: clean
clean:
	rm -rf ./gokvm ./golangci-lint bzImage* vmlinux* CLOUDHV.fd _linux *_string.go

.PHONY: qemu
qemu: initrd bzImage
	qemu-system-x86_64 -kernel ./bzImage -initrd ./initrd --nographic --enable-kvm \
		--append "root=/dev/ram rw console=ttyS0 rdinit=/init" --enable-kvm
