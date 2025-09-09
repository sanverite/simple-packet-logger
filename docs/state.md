# State Machine and Snapshots

## Agent State

Allowed transitions:

- inactive -> starting | active
- starting -> active | error | inactive
- active -> degraded | stopping | error
- degraded -> active | stopping | error
- stopping -> inactive | error
- error -> inactive | starting

Rules:

- First transition to `active` sets `StartedAt`.
- Transition to `inactive` clears `StartedAt`.
- `Uptime()` derives from `StartedAt` and wall-clock time.

## Snapshots

- TUNSnapshot: interface view (name, up, mtu, local/peer IPs)
- RouteSnapshot: default route, LAN CIDRs, bypass host routes, original gateway
- Tun2SocksSnapshot: PID, uptime seconds, TCP/UDP health bits
- ProbeSummary: reachability, handshake/connect success, UDP support, latencies, features, warnings

All snapshots are replaced atomically via Update methods and exposed via deep-copied `Snapshot`.

