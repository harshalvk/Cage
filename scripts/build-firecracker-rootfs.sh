#!/usr/bin/env bash
set -euo pipefail

# Builds a Firecracker-compatible rootfs image via debootstrap, with the
# Cage guest agent injected and every gotcha discovered during initial
# development already accounted for (devpts permissions, root autologin,
# a properly sized filesystem).
#
# Usage: ./scripts/build-firecracker-rootfs.sh <template-slug> [size-mb]
# Example: ./scripts/build-firecracker-rootfs.sh base 2048

TEMPLATE_SLUG="${1:?Usage: $0 <template-slug> [size-mb]}"
SIZE_MB="${2:-2048}"

: "${FIRECRACKER_ROOTFS_DIR:?Set FIRECRACKER_ROOTFS_DIR before running this script}"

WORKDIR="$(mktemp -d)"
BUILD_TREE="$WORKDIR/rootfs-build"
IMAGE_PATH="$WORKDIR/rootfs.ext4"
MOUNT_POINT="$WORKDIR/mount"
GUEST_AGENT_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../guest-agent" && pwd)"

cleanup() {
  if mountpoint -q "$MOUNT_POINT" 2>/dev/null; then
    echo "→ Cleaning up: unmounting $MOUNT_POINT"
    sudo umount "$MOUNT_POINT"
  fi
  sudo rm -rf "$WORKDIR"
}
trap cleanup EXIT

echo "=== Building rootfs for template '$TEMPLATE_SLUG' (${SIZE_MB}MB) ==="

echo "→ Step 0/8: Verifying sudo access..."
if ! sudo -v; then
  echo "ERROR: this script requires sudo access" >&2
  exit 1
fi

echo "→ Step 1/8: Checking for debootstrap..."
if ! command -v debootstrap >/dev/null 2>&1; then
  echo "  debootstrap not found, installing..."
  sudo apt update && sudo apt install -y debootstrap
fi

echo "→ Step 2/8: Bootstrapping base Ubuntu jammy system (this takes a few minutes)..."
sudo debootstrap --arch=amd64 jammy "$BUILD_TREE" http://archive.ubuntu.com/ubuntu

echo "→ Step 3/8: Setting hostname..."
echo "cage-sandbox" | sudo tee "$BUILD_TREE/etc/hostname" > /dev/null

echo "→ Step 4/8: Fixing devpts permissions (ptmxmode=666 — without this, PTY sessions fail silently)..."
echo "devpts /dev/pts devpts gid=5,mode=620,ptmxmode=666 0 0" | sudo tee -a "$BUILD_TREE/etc/fstab" > /dev/null

echo "→ Step 5/8: Enabling root autologin on serial console..."
sudo mkdir -p "$BUILD_TREE/etc/systemd/system/serial-getty@ttyS0.service.d"
sudo tee "$BUILD_TREE/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf" > /dev/null <<'EOF'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

echo "→ Step 6/8: Building and injecting the Cage guest agent..."
(cd "$GUEST_AGENT_SRC" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$WORKDIR/guest-agent" .)
BUILT_HASH=$(md5sum "$WORKDIR/guest-agent" | awk '{print $1}')
echo "  built guest-agent, md5=$BUILT_HASH"

sudo mkdir -p "$BUILD_TREE/usr/local/bin"
sudo cp "$WORKDIR/guest-agent" "$BUILD_TREE/usr/local/bin/guest-agent"
sudo chmod +x "$BUILD_TREE/usr/local/bin/guest-agent"

INJECTED_HASH=$(md5sum "$BUILD_TREE/usr/local/bin/guest-agent" | awk '{print $1}')
if [ "$BUILT_HASH" != "$INJECTED_HASH" ]; then
  echo "  ERROR: injected guest-agent hash ($INJECTED_HASH) does not match built hash ($BUILT_HASH)" >&2
  exit 1
fi
echo "  verified: injected binary matches freshly built binary"

sudo mkdir -p "$BUILD_TREE/etc/systemd/system"
sudo tee "$BUILD_TREE/etc/systemd/system/guest-agent.service" > /dev/null <<'EOF'
[Unit]
Description=Cage guest agent
After=network.target

[Service]
ExecStart=/usr/local/bin/guest-agent
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOF
sudo mkdir -p "$BUILD_TREE/etc/systemd/system/multi-user.target.wants"
sudo ln -sf /etc/systemd/system/guest-agent.service \
  "$BUILD_TREE/etc/systemd/system/multi-user.target.wants/guest-agent.service"

echo "→ Step 7/8: Verifying dpkg is functional before packaging..."
if [ ! -f "$BUILD_TREE/var/lib/dpkg/status" ]; then
  echo "  ERROR: /var/lib/dpkg/status missing — debootstrap did not produce a working dpkg system" >&2
  exit 1
fi
echo "  verified: dpkg/status present"

echo "→ Step 8/8: Packaging into a ${SIZE_MB}MB ext4 image..."
dd if=/dev/zero of="$IMAGE_PATH" bs=1M count="$SIZE_MB" status=none
mkfs.ext4 -q "$IMAGE_PATH"

mkdir -p "$MOUNT_POINT"
sudo mount -o loop "$IMAGE_PATH" "$MOUNT_POINT"
sudo cp -a "$BUILD_TREE/." "$MOUNT_POINT/"

# Final verification INSIDE the mount, before unmounting — this exact
# check would have caught nearly every stale-copy bug hit during
# development of this feature.
FINAL_HASH=$(md5sum "$MOUNT_POINT/usr/local/bin/guest-agent" | awk '{print $1}')
if [ "$FINAL_HASH" != "$BUILT_HASH" ]; then
  echo "  ERROR: final packaged guest-agent hash mismatch after copy into image" >&2
  exit 1
fi
if ! grep -q "ptmxmode=666" "$MOUNT_POINT/etc/fstab"; then
  echo "  ERROR: devpts fix missing from packaged image's fstab" >&2
  exit 1
fi
echo "  verified: guest-agent and fstab fix both present in final image"

sudo umount "$MOUNT_POINT"

DEST="$FIRECRACKER_ROOTFS_DIR/${TEMPLATE_SLUG}.ext4"
mkdir -p "$FIRECRACKER_ROOTFS_DIR"
cp "$IMAGE_PATH" "$DEST"

echo ""
echo "=== Done ==="
echo "Rootfs for template '$TEMPLATE_SLUG' written to: $DEST"
echo "Remember to set firecracker_rootfs_slug = '$TEMPLATE_SLUG' for this template in Postgres if it's a new one."