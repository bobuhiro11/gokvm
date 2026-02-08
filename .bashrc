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

# Start HTTP server early so the test retry loop gets
# a response (even 404) instead of "Connection refused".
srvfiles -h 0.0.0.0 -p 80 > /tmp/srvfiles.log 2>&1 &
echo "srvfiles started as PID=$!"

# If /dev/vda is formatted as ext2, mount as read-only to avoid
# inadvertent fs corruption. If you want to write, please remount.
# Retry up to 30s because the virtio block device may not be
# ready immediately when .bashrc runs.
n=0
while [ $n -lt 30 ]; do
  if [ "$(hexdump -e '/1 "%x"' -s 0x0000438 \
      -n 2 /dev/vda 2>/dev/null)" = "53ef" ]
  then
    mkdir -p /mnt/dev_vda
    mount -o ro /dev/vda /mnt/dev_vda
    ls -la /mnt/dev_vda
    break
  fi
  n=$((n + 1))
  sleep 1
done
ps

echo 'loading .bashrc finished.'
