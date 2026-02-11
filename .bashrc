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

# Diagnostics: show virtio driver probe status
echo "=== dmesg virtio ==="
dmesg | grep -i -E "virtio|vd[a-z]" || true
echo "=== /proc/partitions ==="
cat /proc/partitions 2>&1 || true
echo "=== ls /dev/vd* ==="
ls -la /dev/vd* 2>&1 || true
echo "=== lsblk ==="
lsblk 2>&1 || true
echo "=== end diagnostics ==="

# Give the kernel time to finish virtio-blk driver probe
sleep 2

# Mount /dev/vda as ext2 read-only.
# Retry up to ~60s because the virtio block device may
# not be ready immediately when .bashrc runs.
n=0
mounted=0
while [ $n -lt 60 ]; do
  echo "mount attempt $n"
  mkdir -p /mnt/dev_vda
  if mount -t ext2 -o ro /dev/vda /mnt/dev_vda 2>&1; then
    echo "mount succeeded on attempt $n"
    ls -la /mnt/dev_vda
    mounted=1
    break
  fi
  n=$((n + 1))
  sleep 1
done

if [ "$mounted" -eq 0 ]; then
  echo "WARNING: /dev/vda mount failed after 60 attempts"
  echo "=== final dmesg virtio ==="
  dmesg | grep -i -E "virtio|vd[a-z]" || true
fi

# Start HTTP server AFTER mount so the first 200 OK
# response already contains the mounted content.
srvfiles -h 0.0.0.0 -p 80 > /tmp/srvfiles.log 2>&1 &
echo "srvfiles started as PID=$!"
ps

echo 'loading .bashrc finished.'
