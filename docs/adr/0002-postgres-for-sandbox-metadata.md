# 0002. Use Postgres for sandbox metadata storage

Date: 2026-07-10

## Status

Accepted

## Context

Sandbox metadata (ID, container ID, status, timestamps, template, API keys) started out in an in-memory Go map for simplicity while building the initial REST API. This has an obvious limitation: a server restart loses all record of running sandboxes, leaving orphaned Docker containers with no corresponding metadata, and the service can't run as more than one replica.

Options considered:
- **Stay in-memory, add periodic disk snapshotting** — avoids running a database at all, but reinvents durability and doesn't solve multi-replica coordination.
- **SQLite** — simple, file-based, no separate service to run — but doesn't scale to multiple app replicas writing concurrently, and doesn't match what a real deployment would look like.
- **Postgres** — a real client-server database, matches what an actual production deployment would use, supports concurrent access from multiple app instances, and has mature Go tooling (`pgx`) and migration tooling (`golang-migrate`) that are valuable to learn regardless of this specific project.

## Decision

Use Postgres, accessed via `pgx` directly (no ORM), with schema managed through versioned `golang-migrate` migrations.

## Consequences

- Sandbox metadata survives server restarts; a startup reconciliation step (see `internal/reconcile`) resolves any drift between what the database believes and what Docker actually reports.
- Requires a running Postgres instance for local development (provided via `docker-compose.yml`) and in CI (via testcontainers) — adds a dependency that a pure in-memory or SQLite approach wouldn't have.
- Using raw `pgx` instead of an ORM (GORM, Ent) means every new field requires manually updating `INSERT`/`SELECT`/`Scan` calls in `internal/store` — more boilerplate, but every query is explicit and visible, which was a deliberate tradeoff for learning purposes (see also: no ADR needed for "why not an ORM," since it's addressed here).
- Schema changes must go through a new migration file, never hand-edited into an existing one — this is enforced by convention, not tooling, so care is needed when contributing.