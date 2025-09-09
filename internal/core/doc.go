// Package core owns the daemon's internal state and lifecycle.
//
// Overview
//
// The core package models the daemon as a simple state machine plus a set of
// snapshots describing sub-systems (TUN, routes, tun2socks, last probe).
// It provides a single concurrency boundary: methods on *State.
//
// Concurrency & Safety
//
// State is safe for concurrent use. Read access is via GetSnapshot(), which
// returns a deep copy suitable for use without further locking. Mutation is
// done via narrow UpdateXxx methods and SetAgentState(), each holding the
// internal lock briefly. Callers must never take the lock directly.
//
// Lifecycle
//
// AgentState reflects the coarse lifecycle:
//   inactive -> starting | active
//   starting -> active | error | inactive
//   active   -> degraded | stopping | error
//   degraded -> active | stopping | error
//   stopping -> inactive | error
//   error    -> inactive | starting
//
// SetAgentState enforces these transitions. On the first transition to Active,
// startedAt is set. Transition to Inactive clears startedAt. Uptime derives
// from startedAt.
//
// Snapshots
//
// - TUNSnapshot: interface name, up flag, MTU, local/peer IPs
// - RouteSnapshot: default via, LAN CIDRs, bypass hosts, original gateway
// - Tun2SocksSnapshot: PID, uptime sec, TCP/UDP health
// - ProbeSummary: SOCKS reachability and capabilities, with timings
//
// Update methods replace the entire snapshot atomically to avoid partial-state
// ambiguity. The API layer consumes snapshot copies to serve JSON.
package core

