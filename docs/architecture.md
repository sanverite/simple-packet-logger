# Architecture

## High-Level

- Control Plane (this repo):
  - API server (`internal/api`): JSON over HTTP for health, status, and orchestration.
  - State (`internal/core`): thread-safe, snapshot-based model of the daemon and subsystems.

- Data Plane (planned):
  - TUN interface (macOS): utun device configured by the daemon.
  - tun2socks process: converts captured IP packets to SOCKS requests.
  - Upstream SOCKS proxy (Debian): logs and forwards traffic; separate deployment.

## Flow (Target)

1. Probe proxy: Validate reachability and capabilities (TCP, SOCKS greeting, CONNECT, UDP).
2. Discover environment: Default gateway, local interfaces, LAN CIDRs, bypass host list.
3. Create TUN: Allocate utun, set MTU and IPs, mark up.
4. Route swap: Replace default route to TUN; add host routes for bypasses (router, proxy).
5. Start tun2socks: Point to proxy, supervise process, expose health and PID.
6. Operate: Monitor connectivity, surface state via /status; emit metrics.
7. Stop: Restore routes, stop tun2socks, tear down TUN; idempotent and transactional.

## Design Tenets

- Separation of concerns: core state vs API vs platform utilities.
- Transactional orchestration: each step has a rollback; partial failures leave system healthy.
- Defensive read model: snapshots are deep-copied for consumers.
- Versioned API: stable external contract; internal types can evolve.

## macOS Specifics (Planned)

- TUN: utun via system calls or a library; assign point-to-point IPs (e.g., 10.0.0.1/30).
- Routing: `route`/`scutil` or net APIs; host routes for proxy/gateway to bypass TUN.
- Permissions: likely requires elevated privileges for TUN and routing operations.
- Rollback: record OriginalGateway and interface settings; restore on stop or failure.

## Process Supervision

- Start tun2socks as a child process; capture stdout/stderr for diagnostics.
- Track uptime and basic health (socket checks) in Tun2SocksSnapshot.
- Restart policy with backoff on failure; bounded retries; surface warnings.

