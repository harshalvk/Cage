# 0003. Use tar archives for file transfer to/from sandboxes

Date: 2026-07-09

## Status

Accepted

## Context

The Docker Engine API does not expose a "write this single file into a container" endpoint. The only mechanisms available (`CopyToContainer`/`CopyFromContainer`) operate on tar archive streams — you build a tar archive containing the file(s) you want to write, and Docker extracts it inside the container's filesystem at a given path; reading works in reverse, returning a tar stream even for a single file.

There is no simpler alternative within the Docker SDK itself. The realistic alternatives were:
- **Exec-based file writing** (e.g. `sh -c "cat > /path"` with content piped over stdin via exec) — avoids tar entirely, but is fragile for binary content and awkward for larger files, and doesn't compose as cleanly with the existing `ExecCommand` plumbing.
- **Tar archive construction in memory**, which is what the Docker SDK itself expects and documents.

## Decision

Build single-file tar archives in memory (via `archive/tar` and `bytes.Buffer`) for `WriteFile`, and unwrap the single-entry tar stream returned by `CopyFromContainer` for `ReadFile`.

## Consequences

- No external dependencies needed — `archive/tar` is part of the Go standard library.
- Current implementation only handles plain UTF-8 text content passed as a JSON string field; binary file support (images, binaries, zips) would require switching the API's request format to base64-encoded content or `multipart/form-data`, which hasn't been implemented yet.
- No path validation currently exists on the write path — a client can write to any absolute path inside the sandbox's filesystem. Since this only affects the isolated container's own filesystem (not the host), this is a lower-severity gap than it would be otherwise, but is worth restricting (e.g. to a `/home/user`-style sandbox working directory) before treating this as production-ready.