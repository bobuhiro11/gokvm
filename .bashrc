#!/bin/sh

echo 'loading .bashrc started.'

mount -t proc none /proc
mount --bind /dev /dev
mount --bind /dev/pts /dev/pts

GUEST_IPV4_ADDR=$(cat /proc/cmdline | grep -o "gokvm.ipv4_addr=\S*" | cut -d= -f2)
if [ -n "$GUEST_IPV4_ADDR" ]
then
  ip link set eth0 up
  ip addr add $GUEST_IPV4_ADDR dev eth0
fi

# If /dev/vda is formatted as ext2, mount as read-only to avoid
# inadvertent fs corruption. If you want to write, please remount.
if [ "$(hexdump  -e '/1 "%x"' -s 0x0000438 -n 2 /dev/vda)" = "53ef" ]
then
  mkdir -p /mnt/dev_vda
  mount -o ro /dev/vda /mnt/dev_vda
  ls -la /mnt/dev_vda
fi

nohup srvfiles -h 0.0.0.0 -p 80 &
ps

echo 'loading .bashrc finished.'
