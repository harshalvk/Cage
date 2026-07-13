# 0004. Hash API keys before storing, never store raw keys

Date: 2026-07-11

## Status

Accepted

## Context

Cage needed a way to authenticate API clients before exposing sandbox creation/exec/file endpoints publicly. The simplest implementation would generate a random key, store it directly in Postgres, and compare incoming `Authorization` headers against the stored value directly.

Storing raw keys means that any read access to the `api_keys` table (a database backup, a leaked credential, an internal tool with overly broad query access) immediately exposes every valid credential in the system, with no additional work required by an attacker — equivalent to storing user passwords in plaintext.

## Decision

Generate a high-entropy random key (32 bytes, hex-encoded, prefixed `cage_` for identifiability in logs/secret scanners), show it to the user exactly once at creation time, and store only its SHA-256 hash. Validation re-hashes the incoming key and compares hashes, never raw values.

SHA-256 (not a slow/salted hash like bcrypt) was chosen deliberately: API keys are high-entropy random tokens, not human-chosen passwords, so they aren't vulnerable to dictionary/brute-force guessing the way passwords are — a fast deterministic hash is appropriate here and keeps lookups cheap (a plain indexed equality query), which a slow password-hashing algorithm would make impractical for this use case.

## Consequences

- A database leak exposes only key hashes, not usable credentials — an attacker cannot reconstruct the original key from the hash.
- The raw key is unrecoverable once shown — if a user loses it, they must generate a new one (revocation model, not password-reset model). This is consistent with how most API key systems work in practice (Stripe, GitHub tokens, etc).
- Revocation is a simple `UPDATE api_keys SET revoked_at = now()`; no code change needed since `ValidateAPIKey` already filters on `revoked_at IS NULL`.
- Per-key scoping (rate limits, usage tracking, associating created sandboxes with the key that made them) is not yet implemented — the `api_keys` table currently only supports pass/fail validation, not fine-grained authorization.