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

ProbeSummary semantics:
- `reachable`: true if TCP connect to the proxy endpoint succeeded.
- `socks_ok`: true if greeting (and RFC 1929 username/password auth if required) succeeded.
- `connect_ok`: true if SOCKS CONNECT to the target succeeded.
- `udp_ok`: true if a minimal UDP ASSOCIATE exchange succeeded.
- `latencies_ms`: per-step timings (ms). Keys: `tcp_connect`, `socks_handshake`, `connect`, and `udp_associate` (when applicable).
- `features`:
  - `auth`: "none" or "userpass" (as negotiated).
  - `ipv6`: true only if CONNECT to an IPv6 literal succeeded (proxy supports IPv6 egress).
  - `udp`: reserved for richer UDP validation (false by default from the simple probe).

All snapshots are replaced atomically via Update methods and exposed via deep-copied `Snapshot`.
