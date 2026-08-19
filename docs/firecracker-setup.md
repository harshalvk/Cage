# Running Cage with the Firecracker Backend

Cage supports two isolation backends, selected via `ISOLATION_BACKEND` in
your `.env`: `docker` (default) and `firecracker`. This document covers
what Firecracker needs that Docker doesn't.

## Prerequisites

- **Linux with KVM access.** Firecracker requires hardware virtualization
  (`/dev/kvm`). This works on a real Linux host, or on WSL2 if nested
  virtualization is enabled and exposed — see the WSL2 section below if
  you hit issues.
- **The `firecracker` binary** on your `PATH` or referenced by
  `FIRECRACKER_BIN`.
- **A Linux kernel image** (`vmlinux.bin`) — a bare, uncompressed kernel
  binary, not a regular distro kernel package.
- **A rootfs image per template**, with the Cage guest agent already
  injected (see below) — one `.ext4` file per template slug under
  `FIRECRACKER_ROOTFS_DIR`.

## Verifying KVM is available

```bash
ls -la /dev/kvm
groups | grep kvm   # confirm your user can actually use it, not just that it exists
```

If `/dev/kvm` doesn't exist under WSL2: confirm you're on WSL2 (not WSL1,
`wsl -l -v`), confirm `Virtual Machine Platform` is enabled in Windows
Features, and check `%UserProfile%\.wslconfig` has `nestedVirtualization=true`
under `[wsl2]`. This is hardware/BIOS-dependent and not guaranteed to work
on every machine.

## Environment variables

```bash
ISOLATION_BACKEND=firecracker
FIRECRACKER_BIN=/usr/local/bin/firecracker
FIRECRACKER_KERNEL=/var/lib/cage/vmlinux.bin
FIRECRACKER_ROOTFS_DIR=/var/lib/cage/rootfs-base
FIRECRACKER_RUN_DIR=/var/lib/cage/fc-run
```

## Building a template's rootfs (with the guest agent injected)

Every template needs its own base rootfs at
`$FIRECRACKER_ROOTFS_DIR/<slug>.ext4`, containing a statically-linked copy
of the guest agent (`guest-agent/`) autostarted via systemd.

```bash
# 1. Cross-compile the guest agent
cd guest-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o guest-agent .

# 2. Mount your base rootfs image and inject it
sudo mkdir -p /tmp/rootfs-mount
sudo mount -o loop <your-base>.ext4 /tmp/rootfs-mount
sudo cp guest-agent /tmp/rootfs-mount/usr/local/bin/guest-agent
sudo chmod +x /tmp/rootfs-mount/usr/local/bin/guest-agent

# 3. Add the systemd unit (see guest-agent/guest-agent.service for the file)
sudo cp guest-agent/guest-agent.service \
  /tmp/rootfs-mount/etc/systemd/system/guest-agent.service
sudo mkdir -p /tmp/rootfs-mount/etc/systemd/system/multi-user.target.wants
sudo ln -sf /etc/systemd/system/guest-agent.service \
  /tmp/rootfs-mount/etc/systemd/system/multi-user.target.wants/guest-agent.service

# 4. Unmount and place it where Cage expects it
sudo umount /tmp/rootfs-mount
cp <your-base>.ext4 $FIRECRACKER_ROOTFS_DIR/<template-slug>.ext4
```

There is currently no automated tooling for this — building a new
template's rootfs is a manual process. Automating it (a
`make firecracker-build-template slug=X` target) is a known gap, not yet
built.

## Building the base rootfs (debootstrap)

The Firecracker quickstart demo rootfs is not suitable as a real sandbox
base — it lacks a functioning dpkg/apt system entirely. Build a proper one
instead:

\`\`\`bash
sudo apt install -y debootstrap
sudo debootstrap --arch=amd64 jammy ~/firecracker/rootfs-build http://archive.ubuntu.com/ubuntu
echo "cage-sandbox" | sudo tee ~/firecracker/rootfs-build/etc/hostname
echo "devpts /dev/pts devpts gid=5,mode=620,ptmxmode=666 0 0" | sudo tee -a ~/firecracker/rootfs-build/etc/fstab
# + guest agent injection + systemd unit, same as before
# + package into ext4, size generously (2GB+) up front
\`\`\`

Known gotchas this build must account for:
- `ptmxmode=666` in `/etc/fstab` — without it, PTY allocation fails silently and shell sessions get an EOF on connect.
- Root auto-login via a `serial-getty@ttyS0.service.d` override, since debootstrap doesn't configure this by default.
- The kernel (`vmlinux.bin`, 4.14) is older than jammy's userspace expects — works for basic use, watch for issues with newer syscall-dependent tools.

## Registering a template for Firecracker

Templates need `firecracker_rootfs_slug` set in Postgres (see migration
`add_firecracker_rootfs_slug`) — a template with only `image` set (the
Docker field) will fail with a clear error if you try to use it while
`ISOLATION_BACKEND=firecracker`, rather than silently misbehaving.

```sql
UPDATE templates SET firecracker_rootfs_slug = 'my-slug' WHERE slug = 'my-template';
```

## Known limitations

- **No networking.** Sandboxes have no network access at all — exec and
  file transfer work over vsock, which doesn't need it, but anything
  requiring internet access inside a sandbox (`pip install`, `curl`) will
  fail. See ADR 0012 for why this was deprioritized.
- **Both backends cannot run simultaneously.** `ISOLATION_BACKEND` is a
  single global switch, not a per-sandbox choice.
- **Snapshot/restore is tied to your Firecracker binary version.** Don't
  upgrade the `firecracker` binary while sandboxes are paused — see ADR
  0015.
- **CI does not test this backend.** GitHub-hosted runners do not expose
  `/dev/kvm`; only the mocked unit tests in `internal/firecracker` run in
  CI. Real validation requires a local run against real KVM.