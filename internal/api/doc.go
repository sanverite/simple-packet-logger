// Package api exposes a small HTTP control-plane for the daemon.
//
// Separation of Concerns
//
// The api package defines public JSON types (decoupled from core), maps
// core snapshots to JSON, and hosts an HTTP server with minimal middleware.
// The core package remains unaware of HTTP or JSON.
//
// Versioning
//
// All routes are versioned under /v1. Non-breaking additions extend types,
// while breaking changes require a new prefix (/v2).
//
// Server
//
// NewServer wires handlers onto a ServeMux and configures timeouts. Start()
// runs ListenAndServe() in a goroutine; Stop() performs graceful shutdown.
// Middleware sets JSON content type and logs method/path/duration.
//
// Error Model
//
// APIError uses a string message and a timestamp in RFC3339. Handlers validate
// methods and respond with 405 where appropriate.
//
// Current Endpoints
//
// - GET /v1/healthz: basic liveness/readiness
// - GET /v1/status: maps core.Snapshot into stable JSON (see docs/api.md)
package api

