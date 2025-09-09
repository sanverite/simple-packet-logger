// Command agent runs the local control-plane HTTP server.
//
// Usage:
//
//   agent -listen 127.0.0.1:8787 -shutdown-secs 5
//
// Flags:
//   -listen          HTTP bind address (default 127.0.0.1:8787)
//   -shutdown-secs   graceful shutdown timeout in seconds (default 5)
//
// Behavior:
//
// Initializes core state, starts the API server, and blocks on SIGINT/SIGTERM
// for graceful shutdown. The binary intentionally avoids daemonizing itself;
// packaging as a launchd service is recommended for persistence.
package main

