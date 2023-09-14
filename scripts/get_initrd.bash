#!/bin/bash -x
# ----------------------------------------------------------------------
# get_initrd.bash - download or build initrd
# ----------------------------------------------------------------------

# Download pre-compiled initrd
curl -s -O -L -C - --retry 5 \
  https://github.com/bobuhiro11/bins/raw/main/gokvm/initrd

# Check that initrd was compiled with latest get_initrd.bash.
md5sum=$(md5sum ./scripts/get_initrd.bash | awk '{print $1}')
lsinitramfs initrd | grep $md5sum

# If needed, we build initrd in local
if [ $? -ne 0 ]; then
  # because a go-based VMM deserves a go initrd.
  # but we include bash (because people like it) and other handy tools.
  # we include the local host tools; the u-root -files command will arrange to bring
  # in all the needed .so
  # You need to have installed the u-root command.
  # GOPATH needs to be set.
  # Something weird here: if I use $SHELL in this it expands to /bin/sh *in this makefile*, but not outside. WTF?
  pwd=$(pwd)
  (cd ${GOPATH}/src/github.com/u-root/u-root && \
    u-root \
    -defaultsh `which bash` \
    -o ${pwd}/initrd \
    -files `which ethtool` \
    -files `which lspci` \
    -files `which lsblk` \
    -files `which hexdump` \
    -files `which mount` \
    -files `which bash` \
    -files `which nohup` \
    -files `which clear` \
    -files `which tic` \
    -files `which awk` \
    -files `which grep` \
    -files `which cut` \
    -files "/usr/share/terminfo/l/linux-c:/usr/share/terminfo/l/linux" \
    -files "/usr/share/misc/pci.ids" \
    -files "${pwd}/.bashrc:.bashrc" \
    -files "/dev/null:$md5sum" \
    core boot github.com/u-root/u-root/cmds/exp/srvfiles)
fi
