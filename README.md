# Cage 
<img width="1788" height="464" alt="image" src="https://github.com/user-attachments/assets/d463f5f5-9029-4a4f-8c18-2f03d5339ed4" />

**Cage** is an open-source, self-hostable clone of [E2B](https://e2b.dev) — a backend service for spinning up secure, isolated sandboxes to run untrusted or AI-generated code. Built in Go.

> ⚠️ **Work in progress.** Cage is a learning project and is not production-ready. Currently uses Docker containers as the isolation layer, with plans to explore Firecracker microVMs.

## What is Cage?

Cage lets you programmatically create isolated environments ("sandboxes"), run commands inside them, and tear them down — all through a simple REST API. Think of it as the infrastructure layer you'd use to let an AI agent safely execute code without touching your host system.

```bash
# create a sandbox
curl -X POST http://localhost:8080/sandboxes

# → {"id":"a1b2c3...","status":"running","created_at":"2026-07-08T12:00:00Z"}
```

## Features

- [x] Sandbox lifecycle management (create, list, get, delete)
- [x] Docker-backed isolation
- [x] Command execution inside sandboxes (stdout/stderr streaming)
- [x] File upload/download to/from sandboxes
- [x] Persistent storage (Postgres) for sandbox metadata
- [ ] Idle/expiry-based sandbox cleanup
- [ ] API key authentication
- [ ] Custom sandbox templates
- [ ] Pause/resume support
- [ ] Firecracker microVM backend (long-term goal)

## Architecture

Cage exposes a REST API that manages the lifecycle of sandboxes. Each sandbox currently maps 1:1 to a Docker container, with an in-memory (soon Postgres-backed) store tracking metadata.

<img width="6438" height="3579" alt="image" src="https://github.com/user-attachments/assets/5495eb24-523e-47d3-b1a0-b6482a64ec08" />


## Getting Started

### Prerequisites

- Go 1.25+
- Docker + Docker Compose
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI
- [Air](https://github.com/air-verse/air) (optional, for live reload)
- [golangci-lint](https://golangci-lint.run/) (optional, for linting)
- [Lefthook](https://github.com/evilmartians/lefthook) (optional, for git hooks)

### Installation

```bash
git clone https://github.com/<your-username>/cage.git
cd cage
go mod tidy
cp .env.example .env   # fill in real values
make setup             # installs git hooks
make migrate-up         # apply DB schema
```

### Running
**Option A - Full stack via docker compose (postgres + cage, containerized):**
```bash
docker compose up --build
```
**Option B - Local dev (golang on host, postgres in docker, live reload):**
```bash
docker compose up -d cage-postgres
make migrate-up
make dev    # live-reloading dev server
```

The API will be available at `http://localhost:8080`.

## Development

| Command              | Description                          |
|-----------------------|---------------------------------------|
| `make dev`             | Run with live reload (Air)            |
| `make build`           | Build the binary                      |
| `make lint`            | Run golangci-lint                     |
| `make fmt`             | Format code                            |
| `make migrate-up`      | Apply DB migrations                    |
| `make migrate-down`    | Roll back last migration              |
| `make migrate-create name=X` | Create a new migration pair    |
| `make test`            | Run tests                             |

### API Reference

| Method | Endpoint            | Description              |
|--------|----------------------|---------------------------|
| GET    | `/health`             | Health check              |
| POST   | `/sandboxes`          | Create a new sandbox      |
| GET    | `/sandboxes`           | List all sandboxes        |
| GET    | `/sandboxes/{id}`      | Get sandbox details       |
| DELETE | `/sandboxes/{id}`      | Kill and remove a sandbox |

## Project Status

This project is being built incrementally as a learning exercise in Go, container orchestration, and infrastructure design. Progress and design notes are tracked as the project evolves — see [Issues](../../issues) for planned work.

## License

MIT

## Acknowledgements

Inspired by [E2B](https://e2b.dev), an excellent open-source sandbox infrastructure platform. Cage is an independent educational project and is not affiliated with E2B/FoundryLabs.

