# Operations

## Running Locally

- Start: `./agent -listen 127.0.0.1:8787`
- Health: `curl -s localhost:8787/v1/healthz`
- Status: `curl -s localhost:8787/v1/status | jq`

## Logging

- API logs method, path, and latency per request to stderr.
- Future: structured logs with levels and redaction for secrets.

## Shutdown

- SIGINT/SIGTERM triggers graceful HTTP shutdown with a configurable timeout (`-shutdown-secs`).

## Packaging (Planned)

- macOS launchd service (plist) for persistence across reboots.
- Logs to `~/Library/Logs/simple-packet-logger/` or system log.

## Security Considerations

- API binds to localhost by default; do not bind to public interfaces without auth.
- Operations that touch TUN/routing will require elevated privileges (sudo or helper).
- Avoid logging sensitive proxy credentials; redact in logs and API.

