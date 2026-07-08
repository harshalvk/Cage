# 0001. Use Docker instead of Firecracker for sandbox isolation

Date: 2026-07-08

## Status

Accepted

## Context

E2B's production system uses Firecracker microVMs for sandbox isolation — a lightweight KVM-based hypervisor that gives strong security boundaries with fast boot times. This project's goal is to learn how a system like E2B works by building a clone of it.

Firecracker requires bare-metal or nested-virtualization access to KVM, root-level host configuration, and a substantial amount of infrastructure plumbing (jailer, network taps, rootfs images) before you can run a single sandbox. This is a significant amount of up-front complexity that is orthogonal to the actual goal of this project at this stage: understanding sandbox lifecycle management, the API shape, exec/file operations, and orchestration patterns.

Docker containers provide the same conceptual primitives needed to learn these patterns — isolated filesystem, isolated process tree, resource limits, start/stop/pause lifecycle — via a widely available, well-documented SDK that works identically across a laptop and CI runner.

## Decision

Use Docker containers (via the Docker Engine API / Go SDK) as the isolation primitive for sandboxes, instead of Firecracker microVMs.

## Consequences

- Sandbox lifecycle, exec, and file-transfer code can be written and tested on any machine with Docker installed — no bare-metal/KVM requirement, works in GitHub Actions runners out of the box.
- Isolation is weaker than Firecracker: Docker containers share the host kernel, so the security boundary is a namespace/cgroup boundary, not a virtual machine boundary. This is acceptable for a learning project but would need to change before running genuinely untrusted third-party code in production.
- The `SandboxManager` in `internal/sandbox` is the seam where a Firecracker (or gVisor, or Kata Containers) backend could be substituted later, since it already sits behind a `DockerClient` interface for testability — swapping the isolation backend later is a scoped, identifiable piece of work rather than a full rewrite.
- Docker-specific behaviors (multiplexed exec stdout/stderr streams, tar-based file copy, cgroup-freezer pause semantics) are baked into the current implementation and would need to be re-abstracted if a second backend is added.