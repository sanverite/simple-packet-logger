package api

import "time"

// Public JSON types returned by the API. These are intentionally decoupled
// from the internal core types to preserve API stability and allow internal
// refactors without breaking clients.

// StatusResponse is the top-level payload for GET /v1/status.
type StatusResponse struct {
	State       string        `json:"state"`
	StartedAt   string        `json:"started_at"`
	UptimeSec   int64         `json:"uptime_sec"`
	Warnings    []string      `json:"warnings"`
	TUN         TUNView       `json:"tun"`
	Routes      RoutesView    `json:"routes"`
	Tun2Socks   Tun2SocksView `json:"tun2socks"`
	LastProbe   ProbeView     `json:"last_probe"`
	GeneratedAt string        `json:"generated_at"`
}

// TUNView describes the current view of the TUN interface.
type TUNView struct {
	Name    string `json:"name"`
	Up      bool   `json:"up"`
	MTU     int    `json:"mtu"`
	LocalIP string `json:"local_ip"`
	PeerIP  string `json:"peer_ip"`
}

// RoutesView summarizes the routing decisions.
type RoutesView struct {
	DefaultVia      string   `json:"default_via"`
	LanCIDRs        []string `json:"lan_cidrs"`
	BypassHosts     []string `json:"bypass_hosts"`
	ProxyHostRoute  bool     `json:"proxy_host_route"`
	OriginalGateway string   `json:"original_gateway"`
}

// Tun2SocksView summarizes the supervised tun2socks process.
type Tun2SocksView struct {
	PID       int   `json:"pid"`
	UptimeSec int64 `json:"uptime_sec"`
	TCPOk     bool  `json:"tcp_ok"`
	UDPOk     bool  `json:"udp_ok"`
}

// ProbeView summarizes the last proxy probe.
type ProbeView struct {
	Reachable   bool             `json:"reachable"`
	SocksOK     bool             `json:"socks_ok"`
	ConnectOK   bool             `json:"connect_ok"`
	UDPOK       bool             `json:"udp_ok"`
	LatenciesMs map[string]int64 `json:"latencies_ms"`
	Features    ProxyFeatures    `json:"features"`
	LastChecked string           `json:"last_checked"`
	Warnings    []string         `json:"warnings"`
}

// ProxyFeatures reports discovered capabilities.
type ProxyFeatures struct {
	Auth string `json:"auth"` // "none" or "userpass"
	IPv6 bool   `json:"ipv6"`
	UDP  bool   `json:"udp"`
}

// APIError is a standard error payload.
type APIError struct {
	Error     string `json:"error"`
	Timestamp string `json:"timestamp"` // RFC3339
}

// TimeNow abstracts time for tests; overridden in tests.
var TimeNow = func() time.Time { return time.Now() }

// ProbeRequest is the input body for POST /v1/probe.
// It configures a bounded SOCKS5 probe with optional auth and UDP test.
//
// SocksServer is the upstream SOCKS5 proxy endpoint ("host:port").
// TimeoutMS bounds the entire probe (0 = server default).
// Auth holds optional credentials for proxies that require user/pass.
// ConnectTarget is the target used for the CONNECT test ("host:port").
// Empty uses a sensible default.
// UDPTest requests a minimal UDP ASSOCIATE exchange.
type ProbeRequest struct {
	SocksServer   string     `json:"socks_server"`
	TimeoutMS     int        `json:"timeout_ms"`
	Auth          *ProbeAuth `json:"auth,omitempty"`
	ConnectTarget string     `json:"connect_target"`
	UDPTest       bool       `json:"udp_test"`
}

// ProbeAuth captures optional SOCKS5 username/password credentials.
type ProbeAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// StartRequest configures orchestration to route host traffic via TUN + tun2socks.
//
// SocksServer is the upstream SOCKS5 proxy endpoint ("host:port")
// Auth holds optional credentials for proxies that require user/pass.
// MTU to set for the TUN interface. If 0, default will be user (e.g., 1500)
// ConnectTarget used for initial end-to-end verification via CONNECT ("host:port")
// Empty uses a sensible default.
// BypassHosts will be routed outside the TUN (e.g., proxy host, LAN router).
// DryRun performs discovery/probes and reports the plan without making changes.
type StartRequest struct {
	SocksServer   string     `json:"socks_server"`
	Auth          *ProbeAuth `json:"auth,omitempty"`
	MTU           int        `json:"mtu,omitempty"`
	ConnectTarget string     `json:"connect_target"`
	UDP           bool       `json:"udp"`
	BypassHosts   []string   `json:"bypass_hosts"`
	DryRun        bool       `json:"dry_run"`
}

// StartResponse summarizes the orchestration result and current state snapshot.
type StartResponse struct {
	State       string        `json:"state"`
	Warnings    []string      `json:"warnings"`
	TUN         TUNView       `json:"tun"`
	Routes      RoutesView    `json:"routes"`
	Tun2Socks   Tun2SocksView `json:"tun2socks"`
	GeneratedAt string        `json:"generated_at"`
}

// StopRequest tears down orchestration and restores original routes.
type StopRequest struct {
	// Force skips graceful shutdown of tun2socks and proceeds with teardown.
	Force bool `json:"force"`
}

// StopResponse provides a summary after teardown.
type StopResponse struct {
	State       string   `json:"state"`
	Warnings    []string `json:"warnings"`
	GeneratedAt string   `json:"generated_at"`
}
