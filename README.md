# gokvm [![Build Status](https://travis-ci.com/nmi/gokvm.svg?branch=main)](https://travis-ci.com/nmi/gokvm) [![Coverage Status](https://coveralls.io/repos/github/nmi/gokvm/badge.svg?branch=main)](https://coveralls.io/github/nmi/gokvm?branch=main) ![Lines of code](https://img.shields.io/tokei/lines/github/nmi/gokvm) [![Go Reference](https://pkg.go.dev/badge/github.com/nmi/gokvm.svg)](https://pkg.go.dev/github.com/nmi/gokvm) [![Go Report Card](https://goreportcard.com/badge/github.com/nmi/gokvm)](https://goreportcard.com/report/github.com/nmi/gokvm) [![Maintainability](https://api.codeclimate.com/v1/badges/f60e75353f617035d732/maintainability)](https://codeclimate.com/github/nmi/gokvm/maintainability)

gokvm is a hypervisor that uses KVM as an acceleration.
It is implemented completely in the Go language and has no dependencies other than the standard library.
With **only 1.5k lines of code**, it can **boot Linux 5.10**, the latest version at the time, without any modifications.
It includes a naive and simple device emulation for serial consoles, but does not support networking, disks, etc.
The execution environment is limited to the x86-64 Linux environment.
This should be useful for those who are interested in how to use KVM from userland.

**This is an experimental project, so please do not use it in production.**

## CLI

Extract the latest release from the Github Release tab and run it.
Before running, make sure /dev/kvm exists.
You can use existing bzImage and initrd, or you can create them using the Makefile of this project.

```bash
wget https://github.com/nmi/gokvm/releases/download/v0.0.1/gokvm_0.0.1_linux_amd64.tar.gz
tar zxvf gokvm_0.0.1_linux_amd64.tar.gz
./gokvm -k ./bzImage -i ./initrd
```

## Go package

This project includes a thin wrapper for the KVM API using ioctl. Please refer to the following link to use it.

https://pkg.go.dev/github.com/nmi/gokvm

## Reference

Thanks to the many useful resources on KVM, this project was able to boot Linux on a virtual machine.

- [Using the KVM API, lwn.net](https://lwn.net/Articles/658511/)
- [kvmtest.c, lwn.net](https://lwn.net/Articles/658512/)
- [KVM tool](https://git.kernel.org/pub/scm/linux/kernel/git/will/kvmtool.git/about/)
- [kvm-hello-world](https://github.com/dpw/kvm-hello-world)
- [aghosn/kvm.go](https://gist.github.com/aghosn/f72c8e8f53bf99c3c4117f49677ab0b9)
- [KVM HOST IN A FEW LINES OF CODE](https://zserge.com/posts/kvm/)
- [zserge/kvm-host.c](https://gist.github.com/zserge/ae9098a75b2b83a1299d19b79b5fe488)
- [CS 695: Virtualization and Cloud Computing, cse.iitb.ac.in](https://www.cse.iitb.ac.in/~cs695/)
- [The Linux/x86 Boot Protocol, kernel.org](https://www.kernel.org/doc/html/latest/x86/boot.html)
- [Build and run minimal Linux / Busybox systems in Qemu](https://gist.github.com/chrisdone/02e165a0004be33734ac2334f215380e)
- [kvm_cost.go, google/gvisor](https://github.com/google/gvisor/blob/master/pkg/sentry/platform/kvm/kvm_const.go)
- [Serial UART information, www.lammertbies.nl](https://www.lammertbies.nl/comm/info/serial-uart)
