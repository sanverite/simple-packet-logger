// Package probe contains active network probes used by the control-plane.
//
// # Overview
//
// The probe package provides bounded, deterministic checks of upstream
// dependencies. Probes accept a context and enforce a global deadline,
// record per-step latencies, and return explicit errors without retries
// or background goroutines.
//
// # SOCKS5 Probe
//
// ProbeSOCKS validates an upstream SOCKS5 proxy with the following sequence:
//  1. TCP connect to the proxy endpoint (sets Reachable on success).
//  2. SOCKS5 greeting (optionally performs RFC 1929 username/password auth).
//  3. CONNECT to a caller-specified target (domain, IPv4, or IPv6).
//  4. (Optional) UDP ASSOCIATE exchange.
//
// Inputs & Configuration
//
//   - Config.Server:       "host:port" of the SOCKS5 proxy (IPv4/IPv6/domain).
//   - Config.Timeout:      global bound for the entire probe (uses defaults if 0).
//   - Config.Auth:         optional credentials (username/password).
//   - Config.ConnectTarget:"host:port" target for CONNECT (defaults if empty).
//   - Config.UDPTest:      request a minimal UDP ASSOCIATE exchange.
//
// Outputs & Semantics
//
// ProbeSOCKS returns core.ProbeSummary capturing:
//   - Reachable:   true if TCP connect to the proxy succeeded.
//   - SocksOK:     true if greeting (and user/pass, when required) succeeded.
//   - ConnectOK:   true if CONNECT to the target succeeded.
//   - UDPOK:       true if a minimal UDP ASSOCIATE succeeded.
//   - LatenciesMs: per-step timings in ms ("tcp_connect", "socks_handshake",
//     "connect", "udp_associate" when applicable).
//   - Features:    discovered capabilities (Auth method, IPv6 when an IPv6
//     literal CONNECT succeeds). The UDP feature flag is reserved
//     for richer validation and remains false in this minimal probe.
//   - Warnings:    non-fatal anomalies collected during the run.
//   - LastChecked: wall-clock timestamp when the probe completed.
//
// # Error Model
//
// Transport or protocol failures return a non-nil error; the summary still
// includes any partial timings and warnings. Callers can persist the result
// in core.State via UpdateProbe and expose it through the API.
//
// # Implementation Notes
//
// The probe enforces deadlines with context timeouts and per-connection
// SetDeadline, avoids global state, and does not spawn background goroutines.
// It is safe to call concurrently.
package probe
