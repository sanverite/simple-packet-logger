# simple-packet-logger

A local daemon that exposes a control‑plane API to route host traffic via a TUN interface and translate packets into SOCKS using tun2socks for logging and analysis. Initially ships with a status/health API; future endpoints orchestrate TUN setup, routing, and proxy probing.

## Features

- Control API: `GET /v1/healthz`, `GET /v1/status`
- Thread‑safe core state with immutable snapshots
- Graceful HTTP server with sane timeouts and logging
- Clear separation of concerns: `core` (state) vs `api` (HTTP)

Planned:
- `POST /v1/probe`: verify SOCKS reachability and capabilities
- `POST /v1/start`: create TUN, swap default route, launch tun2socks
- `POST /v1/stop`: stop tun2socks, restore routes, tear down TUN
- Metrics, persistence, launchd packaging

## Quick Start

- Build: `go build ./cmd/agent`
- Run: `./agent -listen 127.0.0.1:8787`
- Health: `curl -s localhost:8787/v1/healthz`
- Status: `curl -s localhost:8787/v1/status | jq`

## API Summary

- `GET /v1/healthz`: lightweight readiness/liveness
- `GET /v1/status`: stable JSON view of daemon state

See `docs/api.md` for schemas and examples.

## Project Layout

- `cmd/agent`: main binary, flags, process lifecycle
- `internal/core`: state model, lifecycle, snapshots
- `internal/api`: HTTP server, JSON types, mapping from core
- `docs/`: deep dives (architecture, API, state, operations)

## Requirements

- Go 1.22+ (set a stable version in `go.mod` to match your toolchain)
- macOS target for TUN/route orchestration (Linux proxy server for logging)

## Contributing

- Run `go vet ./...` and `go test ./...` before changes
- Keep package docs (`doc.go`) up to date with behavior
