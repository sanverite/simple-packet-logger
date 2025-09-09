package core

import (
	"errors"
	"sync"
	"time"
)

// AgentState represents the lifecycle state of the daemon.
// The state machine is intentionally small and coarse to keep control
// surface limited and reasoning straightforward. The intended transitions:
//
// inactive -> starting | active
// starting -> active | error | inactive
// active   -> degraded | stopping | error
// degraded -> active | stopping | error
// stopping -> inactive | error
// error    -> inactive | starting
//
// Transitions outside this set are rejected by SetAgentState.
type AgentState string

const (
	StateInactive AgentState = "inactive"
	StateStarting AgentState = "starting"
	StateActive   AgentState = "active"
	StateDegraded AgentState = "degraded"
	StateStopping AgentState = "stopping"
	StateError    AgentState = "error"
)

// ProxyFeatures summarizes server-side capabilities discovered via probes.
// Fields are additive when known; absence should be interpreted as unknown,
// not necessarily false for booleans.
type ProxyFeatures struct {
	// Auth: "none" or "userpass". Additional values may be added later.
	Auth string
	// IPv6: true if proxy supports IPv6 egress connect.
	IPv6 bool
	// UDP: true if proxy supports UDP ASSOCIATE
	UDP bool
}

// ProbeSummary is a condensed view of the last SOCKS proxy probe.
// Times and latencies are captured as observed, without smoothing.
type ProbeSummary struct {
	Reachable   bool             // TCP reachability to proxy endpoint
	SocksOK     bool             // Successful SOCKS5 greeting/handshake
	ConnectOK   bool             // Successful CONNECT to a known egress target
	UDPOK       bool             // Successful UDP ASSOCIATE probe
	LatenciesMs map[string]int64 // e.g., "tcp_connect", "socks_handshake", "connect"
	Features    ProxyFeatures    // Discovered capabilities
	LastChecked time.Time        // Wall clock time of probe
	Warnings    []string         // Non-fatal anomalies observed during probe
}

// TUNSnapshot describes the TUN interface state at a point in time.
type TUNSnapshot struct {
	Name    string // OS interface name (e.g., "utun7" on macOS)
	Up      bool   // Administrative up flag
	MTU     int    // MTU currently set
	LocalIP string // Local (interface) IP assigned to TUN
	PeerIP  string // Peer IP (if point-to-point)
}

// RouteSnapshot summarizes routing decisions captured by the daemon.
// LanCIDRs and BypassHosts are additive lists used to steer routing.
// OriginalGateway is the pre-modification default gateway (used for restore).
type RouteSnapshot struct {
	DefaultVia      string   // Current default route gateway (post-swap)
	LanCIDRs        []string // Detected local/LAN networks to bypass
	BypassHosts     []string // Hosts to bypass (e.g., proxy endpoint, router)
	ProxyHostRoute  bool     // whether proxy endpoint has a pinned host route
	OriginalGateway string   // Default gateway observed before swapping
}

// Tun2SocksSnapshot summarizes the supervized tun2socks process.
type Tun2SocksSnapshot struct {
	PID       int   // OS process ID (0 if not running)
	UptimeSec int64 // Monotonic-ish uptime of the process
	TCPOk     bool  // Health check for TCP path
	UDPOk     bool  // Health check for UDP path
}

// Snapshot is a threadsafe read model returned to the API layer.
// All nested slices/maps are returned as defensive copies, so callers
// may safely retain value without additional locking.
type Snapshot struct {
	AgentState AgentState
	StartedAt  time.Time
	Warnings   []string
	TUN        TUNSnapshot
	Routes     RouteSnapshot
	Tun2Socks  Tun2SocksSnapshot
	LastProbe  ProbeSummary
}

// State holds mutable daemon state with synchronization.
// Use the provided methods to mutate; callers should never take the lock directly.
type State struct {
	mu        sync.RWMutex
	agent     AgentState
	startedAt time.Time
	warnings  []string
	tun       TUNSnapshot
	routes    RouteSnapshot
	tun2socks Tun2SocksSnapshot
	lastProbe ProbeSummary
}

// NewState constructs a default-inactive state.
func NewState() *State {
	return &State{
		agent:    StateInactive,
		warnings: nil,
	}
}

// GetSnapshot returns a deep copy safe for concurrent reads.
func (s *State) GetSnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Defensive copies for slices/maps
	warnings := append([]string(nil), s.warnings...)
	lanCIDRs := append([]string(nil), s.routes.LanCIDRs...)
	bypass := append([]string(nil), s.routes.BypassHosts...)
	latencies := make(map[string]int64, len(s.lastProbe.LatenciesMs))
	for k, v := range s.lastProbe.LatenciesMs {
		latencies[k] = v
	}
	probeWarnings := append([]string(nil), s.lastProbe.Warnings...)

	return Snapshot{
		AgentState: s.agent,
		StartedAt:  s.startedAt,
		Warnings:   warnings,
		TUN:        s.tun,
		Routes: RouteSnapshot{
			DefaultVia:      s.routes.DefaultVia,
			LanCIDRs:        lanCIDRs,
			BypassHosts:     bypass,
			ProxyHostRoute:  s.routes.ProxyHostRoute,
			OriginalGateway: s.routes.OriginalGateway,
		},
		Tun2Socks: s.tun2socks,
		LastProbe: ProbeSummary{
			Reachable:   s.lastProbe.Reachable,
			SocksOK:     s.lastProbe.SocksOK,
			ConnectOK:   s.lastProbe.ConnectOK,
			UDPOK:       s.lastProbe.UDPOK,
			LatenciesMs: latencies,
			Features:    s.lastProbe.Features,
			LastChecked: s.lastProbe.LastChecked,
			Warnings:    probeWarnings,
		},
	}
}

// Uptime returns the wall-clock duration since the daemon entered Active state.
// Returns zero if never started. While stopping/degraded, uptime continues
// from the last start; when transitioning to Inactive, uptime resets to zero.
func (s *State) Uptime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.startedAt.IsZero() {
		return 0
	}
	return time.Since(s.startedAt)
}

// SetStartedAt force-sets the startedAt time. This is useful when restoring
// state from persistence. Prefer to rely on SetAgentState which sets it when
// transitioning to Active.
func (s *State) SetStartedAt(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startedAt = t
}

// AppendWarning adds a non-fatal warning to the state.
func (s *State) AppendWarning(msg string) {
	if msg == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, msg)
}

// ClearWarnings removes all accumulated warnings.
func (s *State) ClearWarnings() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = nil
}

// UpdateTUN replaces the current TUN snapshot with the provided value.
func (s *State) UpdateTUN(t TUNSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tun = t
}

// UpdateRoutes replaces the current routing snapshot with the provided value.
// Callers should pass the complete desired view to avoid partial-state ambiguity.
func (s *State) UpdateRoutes(r RouteSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes = r
}

// UpdateTun2Socks replaces the current tun2socks process snapshot.
func (s *State) UpdateTun2Socks(p Tun2SocksSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tun2socks = p
}

// UpdateProbe replaces the last probe summary with a new value.
// Slices/maps are copied defensively.
func (s *State) UpdateProbe(p ProbeSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lat := make(map[string]int64, len(p.LatenciesMs))
	for k, v := range p.LatenciesMs {
		lat[k] = v
	}
	warns := append([]string(nil), p.Warnings...)

	s.lastProbe = ProbeSummary{
		Reachable:   p.Reachable,
		SocksOK:     p.SocksOK,
		ConnectOK:   p.ConnectOK,
		UDPOK:       p.UDPOK,
		LatenciesMs: lat,
		Features:    p.Features,
		LastChecked: p.LastChecked,
		Warnings:    warns,
	}
}

// ErrInvalidTransition is returned when SetAgentState receives an illegal transition.
var ErrInvalidTransition = errors.New("invalid agent state transition")

// SetAgentState transitions the agent to the next state, enforcing a simple
// state machine. On the first transition to Active, startedAt is set. When
// transitioning to Inactive, startedAt is cleared.
//
// Returns ErrInvalidTransition if the (current -> next) edge is not allowed.
func (s *State) SetAgentState(next AgentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.agent
	if cur == next {
		// Idempotent: no-op
		return nil
	}

	if !allowedTransition(cur, next) {
		return ErrInvalidTransition
	}

	// Handle lifecycle timestamps.
	switch next {
	case StateActive:
		// First activate in a run: set startedAt if zero.
		if s.startedAt.IsZero() {
			s.startedAt = time.Now()
		}

	case StateInactive:
		// Fully reset uptime on full stop.
		s.startedAt = time.Time{}
	}

	s.agent = next
	return nil
}

func allowedTransition(cur, next AgentState) bool {
	switch cur {
	case StateInactive:
		return next == StateStarting || next == StateActive
	case StateStarting:
		return next == StateActive || next == StateError || next == StateInactive
	case StateActive:
		return next == StateDegraded || next == StateStopping || next == StateError
	case StateDegraded:
		return next == StateActive || next == StateStopping || next == StateError
	case StateStopping:
		return next == StateInactive || next == StateError
	case StateError:
		return next == StateInactive || next == StateStarting
	default:
		return false
	}
}

// Reset clears all mutable state back to a fresh NewState, retaining only
// the current AgentState and StartedAt semantics as appropriate. This is
// useful to recover from error conditions while keeping lifecycle context.
//
// If clearLifecycle is true, also resets agent state to Inactive and zeroes
// StartedAt (i.e., full reset).
func (s *State) Reset(clearLifecycle bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if clearLifecycle {
		s.agent = StateInactive
		s.startedAt = time.Time{}
	}

	s.warnings = nil
	s.tun = TUNSnapshot{}
	s.routes = RouteSnapshot{}
	s.tun2socks = Tun2SocksSnapshot{}
	s.lastProbe = ProbeSummary{}
}
