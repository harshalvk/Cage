#!/usr/bin/env bash
set -euo pipefail

# One-time setup for a self-hosted GitHub Actions runner with KVM access,
# for running Firecracker integration tests. Run this on a Linux host (or
# WSL2 with KVM passthrough confirmed working) BEFORE registering it as a
# runner via GitHub's own runner registration script.

echo "→ Checking for KVM access..."
if [ ! -e /dev/kvm ]; then
  echo "ERROR: /dev/kvm does not exist. This host cannot run Firecracker." >&2
  echo "See docs/firecracker-setup.md for KVM prerequisites." >&2
  exit 1
fi
echo "  OK"

echo "→ Installing Firecracker binary..."
FC_VERSION=$(curl -s https://api.github.com/repos/firecracker-microvm/firecracker/releases/latest | grep '"tag_name"' | cut -d'"' -f4)
FC_URL=$(curl -s "https://api.github.com/repos/firecracker-microvm/firecracker/releases/latest" \
  | grep browser_download_url | grep x86_64 | grep -v debug | cut -d '"' -f4)
curl -L -o /tmp/firecracker.tgz "$FC_URL"
tar xzf /tmp/firecracker.tgz -C /tmp
sudo mv /tmp/release-*/firecracker-*-x86_64 /usr/local/bin/firecracker
sudo chmod +x /usr/local/bin/firecracker
echo "  installed $(firecracker --version)"

echo "→ Downloading kernel image..."
sudo mkdir -p /opt/cage-ci/kernel
sudo curl -fsSL -o /opt/cage-ci/kernel/vmlinux.bin \
  https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin

echo "→ Building base rootfs via debootstrap..."
export FIRECRACKER_ROOTFS_DIR=/opt/cage-ci/rootfs-base
sudo mkdir -p "$FIRECRACKER_ROOTFS_DIR"
"$(dirname "${BASH_SOURCE[0]}")/build-firecracker-rootfs.sh" base

echo ""
echo "=== KVM runner prerequisites installed ==="
echo "Set these as environment variables for the GitHub Actions runner service:"
echo "  FIRECRACKER_BIN=/usr/local/bin/firecracker"
echo "  FIRECRACKER_KERNEL=/opt/cage-ci/kernel/vmlinux.bin"
echo "  FIRECRACKER_ROOTFS_DIR=/opt/cage-ci/rootfs-base"
echo ""
echo "Next: register this host as a GitHub Actions self-hosted runner"
echo "  (Settings -> Actions -> Runners -> New self-hosted runner in your repo),"
echo "  using the label 'kvm' when configuring it."