# 0006. Use cmd/ + internal/ package layout

Date: 2026-07-11

## Status

Accepted

## Context

The project began as a flat `package main` with all files (handlers, store, sandbox manager, config, migrations runner, etc.) in the repository root. This worked while the codebase was small, but became difficult to navigate once it grew past roughly ten files — there was no structural signal for which pieces depended on which, and every type lived in the same global namespace.

Go has a widely adopted community convention (not enforced by the toolchain, except for `internal/`) for structuring applications: a `cmd/<binary-name>/main.go` entrypoint per executable, and an `internal/` directory holding implementation packages that only code within the same module is permitted to import.

## Decision

Restructure the project into:
- `cmd/cage/` — the main server entrypoint
- `cmd/genkey/` — a small standalone CLI for generating API keys
- `internal/api`, `internal/auth`, `internal/config`, `internal/db`, `internal/reaper`, `internal/reconcile`, `internal/sandbox`, `internal/store` — implementation packages, one per responsibility

## Consequences

- `internal/` is compiler-enforced: no external module can import these packages, which is appropriate since none of this is intended as a reusable library.
- Cross-package references now require explicit imports and package-qualified names (e.g. `store.Sandbox`, `sandbox.SandboxManager`) instead of bare identifiers — this is more verbose but makes dependencies between subsystems explicit and greppable.
- Multiple binaries (`cage`, `genkey`) can share the same `internal/` packages cleanly, which would not have been possible under a single flat `package main`.
- This refactor was mechanical but not risk-free: local variables/parameters that happened to share a name with an imported package (e.g. a parameter named `store` shadowing the `store` package) caused subtle compile errors during the migration and required renaming — a pattern worth remembering for future package extractions.