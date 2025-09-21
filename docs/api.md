# API

All endpoints are under `/v1`. Content-Type is `application/json; charset=utf-8`.

## Errors

```json
{
  "error": "human-readable message",
  "timestamp": "2025-01-01T00:00:00Z"
}
```

## GET /v1/healthz

- Purpose: Basic liveness/readiness.
- Responses:
  - 200 OK
    ```json
    {"status":"ok","timestamp":"2025-01-01T00:00:00Z"}
    ```
  - 405 Method Not Allowed with APIError body

## GET /v1/status

- Purpose: Thread-safe snapshot of daemon state.
- Response: 200 OK

Schema:
```json
{
  "state": "inactive|starting|active|degraded|stopping|error",
  "started_at": "RFC3339 or empty string",
  "uptime_sec": 0,
  "warnings": ["..."],
  "tun": {
    "name": "utun7",
    "up": true,
    "mtu": 1500,
    "local_ip": "10.0.0.2",
    "peer_ip": "10.0.0.1"
  },
  "routes": {
    "default_via": "192.168.1.1",
    "lan_cidrs": ["192.168.1.0/24"],
    "bypass_hosts": ["192.168.1.1","proxy.example.com"],
    "proxy_host_route": true,
    "original_gateway": "192.168.1.1"
  },
  "tun2socks": {
    "pid": 12345,
    "uptime_sec": 42,
    "tcp_ok": true,
    "udp_ok": false
  },
  "last_probe": {
    "reachable": true,
    "socks_ok": true,
    "connect_ok": true,
    "udp_ok": false,
    "latencies_ms": {
      "tcp_connect": 12,
      "socks_handshake": 5,
      "connect": 20
    },
    "features": {"auth":"none","ipv6":false,"udp":false},
    "last_checked": "2025-01-01T00:00:00Z",
    "warnings": []
  },
  "generated_at": "2025-01-01T00:00:00Z"
}
```

## Future Endpoints

- `POST /v1/probe` (planned):
  - Input:
    ```json
    {
      "socks_server": "host:port",
      "timeout_ms": 3000,
      "auth": {"username": "", "password": ""},
      "connect_target": "example.com:80",
      "udp_test": false
    }
    ```
  - Output: 200 OK with the same schema as "last_probe" in GET /v1/status; also updates internal state.
    ```json
    {
      "reachable": true,
      "socks_ok": true,
      "connect_ok": true,
      "udp_ok": false,
      "latencies_ms": {"tcp_connect": 12, "socks_handshake": 5, "connect": 20, "udp_associate": 9},
      "features": {"auth": "none", "ipv6": false, "udp": false},
      "last_checked": "2025-01-01T00:00:00Z",
      "warnings": []
    }
    ```
  - Errors:
    - 400 Bad Request for invalid inputs (e.g., malformed host:port).
    - 502 Bad Gateway for probe failures (e.g., TCP connect or CONNECT failed), with an APIError body.
    - 405 Method Not Allowed for non-POST methods.
- `POST /v1/start`:
  - Input: `{ "socks_server":"host:port", "mtu":1500, "bypass":["host"], "dry_run":false }`
  - Output: orchestration summary; state transitions.
- `POST /v1/stop`:
  - Input: `{ "force":false }`
  - Output: teardown summary; state transitions.
