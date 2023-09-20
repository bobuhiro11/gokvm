#!/bin/bash -x
# ----------------------------------------------------------------------
# get_kernel.bash - download or build bzImage and vmlinux
# ----------------------------------------------------------------------

bzImage=${1:-bzImage}
vmlinux=${2:-vmlinux}
config=${3:-linux.config}

# Download pre-compiled bzImage
curl -s -O -L -C - --retry 5 \
  https://github.com/bobuhiro11/bins/raw/main/gokvm/${bzImage}

# Download pre-compiled EDK2/CloudHv firmware image (CLOUDHV.fd)
curl -s -O -L -C - --retry 5 \
  https://github.com/bobuhiro11/bins/raw/main/gokvm/CLOUDHV.fd

# Download extract-ikconfig script
curl -s -O -L -C - --retry 5 \
  https://raw.githubusercontent.com/torvalds/linux/master/scripts/extract-ikconfig
chmod a+x ./extract-ikconfig

# Download extract-vmlinux script
curl -s -O -L -C - --retry 5 \
  https://raw.githubusercontent.com/torvalds/linux/master/scripts/extract-vmlinux
chmod a+x ./extract-vmlinux

# Check that bzImage was compiled with linux.config
diff -u <(./extract-ikconfig ${bzImage}) ${config}

# If needed, we build bzImage in local
if [ $? -ne 0 ]; then
  version=$(awk '/Kernel Configuration/ {print $3}' ./linux.config)
  major_version=$(echo $version | awk -F\. '{print $1}')
  curl -s -O -L -C - --retry 5 \
    https://cdn.kernel.org/pub/linux/kernel/v${major_version}.x/linux-${version}.tar.xz \
    -o linux.tar.xz

  tar Jxf ./linux.tar.xz --one-top-level=_linux --strip-components 1
  cp ${config} _linux/.config
  make -j$(nproc) -C _linux
  cp _linux/arch/x86/boot/bzImage ${bzImage}
fi

# Extract vmlinux from bzImage
./extract-vmlinux ${bzImage} > ${vmlinux}