# gokvm [![Build Status](https://travis-ci.com/nmi/gokvm.svg?branch=main)](https://travis-ci.com/nmi/gokvm) [![Coverage Status](https://coveralls.io/repos/github/nmi/gokvm/badge.svg?branch=main)](https://coveralls.io/github/nmi/gokvm?branch=main) [![Go Reference](https://pkg.go.dev/badge/github.com/nmi/gokvm.svg)](https://pkg.go.dev/github.com/nmi/gokvm) [![Go Report Card](https://goreportcard.com/badge/github.com/nmi/gokvm)](https://goreportcard.com/report/github.com/nmi/gokvm) [![Maintainability](https://api.codeclimate.com/v1/badges/f60e75353f617035d732/maintainability)](https://codeclimate.com/github/nmi/gokvm/maintainability)

__This is work in progress project__.

## Features

- Cooperate with Linux KVM
- Pure Golang (use only standard libraries)
- Aimed to run only unmodified linux
- No emuration of Networking, disk, etc.

## How to use the CLI tool

```bash
$ go get github.com/nmi/gokvm
$ gokvm -h
```

## How to use as golang package

Pseudo code is as below:

```go
import "github.com/nmi/gokvm/kvm"

ioportHandler := func (port uint32, isIn bool, value byte) {
  ...
}
kvm.LinuxRun(bzImagePath, initrdPath, ioportHandler)
```

## Reference

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
