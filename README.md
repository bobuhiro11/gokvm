# gokvm [![Build Status](https://travis-ci.com/bobuhiro11/gokvm.svg?branch=main)](https://travis-ci.com/bobuhiro11/gokvm) [![Coverage Status](https://coveralls.io/repos/github/bobuhiro11/gokvm/badge.svg?branch=main)](https://coveralls.io/github/bobuhiro11/gokvm?branch=main) [![code lines](https://sloc.xyz/github/bobuhiro11/gokvm?category=code)](https://sloc.xyz/github/bobuhiro11/gokvm?category=code) [![Go Reference](https://pkg.go.dev/badge/github.com/bobuhiro11/gokvm.svg)](https://pkg.go.dev/github.com/bobuhiro11/gokvm) [![Go Report Card](https://goreportcard.com/badge/github.com/bobuhiro11/gokvm)](https://goreportcard.com/report/github.com/bobuhiro11/gokvm) [![Maintainability](https://api.codeclimate.com/v1/badges/f60e75353f617035d732/maintainability)](https://codeclimate.com/github/bobuhiro11/gokvm/maintainability)


gokvm is a hypervisor that uses KVM as an acceleration.
It is implemented completely in the Go language and has no dependencies other than the standard library.
With **only 1.5k lines of code**, it can **boot Linux 5.10**, the latest version at the time, without any modifications
(see [v0.0.1](https://github.com/bobuhiro11/gokvm/releases/tag/v0.0.1)).
It includes naive and simple device emulation for serial console, virtio-net, and virtio-blk.
The execution environment is limited to the x86-64 Linux environment.
This should be useful for those who are interested in how to use KVM from userland.
The latest version supports the following features:

- [x] kvm acceleration
- [x] multi processors
- [x] serial console
- [x] virtio-net
- [x] virtio-blk

**This is an experimental project, so please do not use it in production.**

![demo](https://raw.githubusercontent.com/bobuhiro11/gokvm/main/demo.gif)

## CLI

Extract the latest release from [the Github Release tab](https://github.com/bobuhiro11/gokvm/releases) and run it.
Before running, make sure /dev/kvm exists.
You can use existing bzImage and initrd, or you can create them using the Makefile of this project.

```bash
tar zxvf gokvm*.tar.gz
./gokvm -k ./bzImage -i ./initrd  # To exit, press Ctrl-a x.
```

## Go package

This project includes a thin wrapper for the KVM API using ioctl. Please refer to the following link to use it.

https://pkg.go.dev/github.com/bobuhiro11/gokvm

## Reference

Thanks to the many useful resources on KVM, this project was able to boot Linux on a virtual machine.

- [The Definitive KVM API Documentation](https://docs.kernel.org/virt/kvm/api.html#)
- [Using the KVM API, lwn.net](https://lwn.net/Articles/658511/)
- [kvmtest.c, lwn.net](https://lwn.net/Articles/658512/)
- [KVM tool](https://git.kernel.org/pub/scm/linux/kernel/git/will/kvmtool.git/about/)
- [kvm-hello-world](https://github.com/dpw/kvm-hello-world)
- [linux kvm-api: types,structures, consts](https://github.com/torvalds/linux/blob/master/include/uapi/linux/kvm.h)
- [aghosn/kvm.go](https://gist.github.com/aghosn/f72c8e8f53bf99c3c4117f49677ab0b9)
- [KVM HOST IN A FEW LINES OF CODE](https://zserge.com/posts/kvm/)
- [zserge/kvm-host.c](https://gist.github.com/zserge/ae9098a75b2b83a1299d19b79b5fe488)
- [CS 695: Virtualization and Cloud Computing, cse.iitb.ac.in](https://www.cse.iitb.ac.in/~cs695/)
- [The Linux/x86 Boot Protocol, kernel.org](https://www.kernel.org/doc/html/latest/x86/boot.html)
- [Build and run minimal Linux / Busybox systems in Qemu](https://gist.github.com/chrisdone/02e165a0004be33734ac2334f215380e)
- [kvm_cost.go, google/gvisor](https://github.com/google/gvisor/blob/master/pkg/sentry/platform/kvm/kvm_const.go)
- [Serial UART information, www.lammertbies.nl](https://www.lammertbies.nl/comm/info/serial-uart)
- [Virtual I/O Device (VIRTIO) Version 1.1](https://docs.oasis-open.org/virtio/virtio/v1.1/csprd01/virtio-v1.1-csprd01.html)
- [rust-vmm/vm-virtio](https://github.com/rust-vmm/vm-virtio/tree/main/crates/virtio-queue)
- [ハイパーバイザの作り方～ちゃんと理解する仮想化技術～ 第１１回 virtioによる準仮想化デバイス その１「virtioの概要とVirtio PCI」](https://syuu1228.github.io/howto_implement_hypervisor/part11.html)
- [ハイパーバイザの作り方～ちゃんと理解する仮想化技術～ 第１２回 virtioによる準仮想化デバイス その２「Virtqueueとvirtio-netの実現」](https://syuu1228.github.io/howto_implement_hypervisor/part12.html)
