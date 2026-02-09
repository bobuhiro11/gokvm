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

# If /dev/vda is formatted as ext2, mount as read-only
# to avoid inadvertent fs corruption.
# Retry up to ~30s because the virtio block device may
# not be ready immediately when .bashrc runs.
# (10 iterations * (2s hexdump timeout + 1s sleep) = 30s)
echo "checking /dev/vda existence..."
ls -la /dev/vda 2>&1 || true

n=0
mounted=0
while [ $n -lt 30 ]; do
  echo "mount attempt $n"
  magic=$(timeout 5 hexdump -e '/1 "%x"' -s 0x0000438 \
      -n 2 /dev/vda 2>/dev/null) || magic=""
  echo "  hexdump magic=$magic"
  if [ "$magic" = "53ef" ]
  then
    mkdir -p /mnt/dev_vda
    mount -o ro /dev/vda /mnt/dev_vda
    echo "mount succeeded on attempt $n"
    ls -la /mnt/dev_vda
    mounted=1
    break
  fi
  n=$((n + 1))
  sleep 1
done

if [ "$mounted" -eq 0 ]; then
  echo "WARNING: /dev/vda mount failed after 30 attempts"
fi

# Start HTTP server AFTER mount so the first 200 OK
# response already contains the mounted content.
srvfiles -h 0.0.0.0 -p 80 > /tmp/srvfiles.log 2>&1 &
echo "srvfiles started as PID=$!"
ps

echo 'loading .bashrc finished.'
