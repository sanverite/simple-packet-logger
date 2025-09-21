package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/sanverite/simple-packet-logger/internal/core"
	"github.com/sanverite/simple-packet-logger/internal/probe"
)

// Constants for route prefixing. Versioning is explicit to allow non-breaking additions.
const (
	APIVersion     = "v1"
	DefaultAddress = "127.0.0.1:8787"
)

// ServerOptions configures the HTTP server.
// Timeouts are conservative defaults suitable for a local control-plane server.
type ServerOptions struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	Logger            *log.Logger
}

// Server hosts the HTTP API for the daemon.
type Server struct {
	http   *http.Server
	state  *core.State
	logger *log.Logger
	opts   ServerOptions
}

// NewServer constructs a new API server bound to the provided State.
// The server does not start listening until Start is called.
func NewServer(state *core.State, opts ServerOptions) *Server {
	if state == nil {
		panic("api.NewServer: state is nil")
	}
	if opts.Addr == "" {
		opts.Addr = DefaultAddress
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	if opts.ReadHeaderTimeout == 0 {
		opts.ReadHeaderTimeout = 2 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 10 * time.Second
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = 60 * time.Second
	}
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}

	mux := http.NewServeMux()
	s := &Server{
		state:  state,
		logger: opts.Logger,
		opts:   opts,
		http: &http.Server{
			Addr:              opts.Addr,
			Handler:           withBasicMiddleware(mux, opts.Logger),
			ReadTimeout:       opts.ReadTimeout,
			ReadHeaderTimeout: opts.ReadHeaderTimeout,
			WriteTimeout:      opts.WriteTimeout,
			IdleTimeout:       opts.IdleTimeout,
			ErrorLog:          opts.Logger,
			BaseContext: func(l net.Listener) context.Context {
				return context.Background()
			},
		},
	}

	// Routes
	mux.HandleFunc("/"+APIVersion+"/healthz", s.handleHealthz)
	mux.HandleFunc("/"+APIVersion+"/status", s.handleStatus)
	mux.HandleFunc("/"+APIVersion+"/probe", s.handleProbe)
	mux.HandleFunc("/"+APIVersion+"/start", s.handleStart)
	mux.HandleFunc("/"+APIVersion+"/stop", s.handleStop)

	return s
}

// Start begins serving HTTP in a background goroutine.
// It returns immediately; use Stop for graceful shutdown.
func (s *Server) Start() {
	go func() {
		s.logger.Printf("api: listening on %s\n", s.http.Addr)
		if err := s.http.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("api: ListenAndServe error: %v", err)
		}
	}()
}

// Stop gracefully shuts down the server, waiting up to ShutdownTimeout.
func (s *Server) Stop(ctx context.Context) error {
	timeout := s.opts.ShutdownTimeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return s.http.Shutdown(ctx)
}

// handleHealthz is a simple readiness/liveness endpoint.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, APIError{
			Error:     "method not allowed",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"timestamp": TimeNow().UTC().Format(time.RFC3339),
	})
}

// handleStatus returns the current daemon snapshot.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, APIError{
			Error:     "method not allowed",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}
	snap := s.state.GetSnapshot()
	resp := FromCoreSnapshot(snap)
	writeJSON(w, http.StatusOK, resp)
}

// handleProbe runs a bounded SOCKS5 probe and returns a ProbeView.
// Method: POST
// Request: ProbeRequest JSON
// Response (200): ProbeView JSON (same shape as "last_probe" in /v1/status)
// Errors:
//   - 400 for invalid inputs (malformed host:port, negative timeout)
//   - 502 for probe failures (TCP connect/handshake/CONNECT/UDP errors), state still updates
func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, APIError{
			Error:     "method not allowed",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// Strict JSON decode with unkown-field rejection.
	var req ProbeRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{
			Error:     "invalid JSON: " + err.Error(),
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// Basic input validation (deeper checks happen inside the probe package).
	if req.SocksServer == "" {
		writeJSON(w, http.StatusBadRequest, APIError{
			Error:     "socks_server is required",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}
	if req.TimeoutMS < 0 {
		writeJSON(w, http.StatusBadRequest, APIError{
			Error:     "timeout_ms must be >= 0",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// Request -> probe.Config mapping with sensible defaults.
	var auth *probe.Auth
	if req.Auth != nil && (req.Auth.Username != "" || req.Auth.Password != "") {
		auth = &probe.Auth{
			Username: req.Auth.Username,
			Password: req.Auth.Password,
		}
	}
	cfg := probe.Config{
		Server:        req.SocksServer,
		Timeout:       time.Duration(req.TimeoutMS) * time.Millisecond,
		Auth:          auth,
		ConnectTarget: req.ConnectTarget,
		UDPTest:       req.UDPTest,
	}

	// Run the probe using the request context; probe also enforces its own deadline.
	summary, err := probe.ProbeSOCKS(r.Context(), cfg)

	// Persist the result regardless of success.
	s.state.UpdateProbe(summary)

	if err != nil {
		// Return a stable error; details available via /v1/status last_probe.warnings.
		writeJSON(w, http.StatusBadGateway, APIError{
			Error:     "probe failed: " + err.Error(),
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// Success: return the probe payload.
	resp := FromProbeSummary(summary)
	writeJSON(w, http.StatusOK, resp)
}

// handleStart begins orchestration to route traffic via TUN + tun2socks.
// Method: POST
// Request: StartRequest JSON
// Response (200): StartResponse JSON
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, APIError{
			Error:     "method not allowed",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	var req StartRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, APIError{
			Error:     "invalid JSON: " + err.Error(),
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// Basic validation; depper checks will live in orchestrator.
	if req.SocksServer == "" {
		writeJSON(w, http.StatusBadRequest, APIError{
			Error:     "socks_server is required",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// Conservative MTU bounds (typical ethernet MTU to jumbo); 0 means "use default".
	if req.MTU < 0 || (req.MTU > 0 && (req.MTU < 576 || req.MTU > 9000)) {
		writeJSON(w, http.StatusBadRequest, APIError{
			Error:     "mtu must be 0 or between 576 and 9000",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	// orchestration todo
	writeJSON(w, http.StatusNotImplemented, APIError{
		Error:     "start not implemented yet",
		Timestamp: TimeNow().UTC().Format(time.RFC3339),
	})
}

// handleStop tears down orchestration and restores routes.
// Method: POST
// Request: StopRequest JSON
// Response (200): StopResponse JSON
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, APIError{
			Error:     "method not allowed",
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	var req StopRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusNotImplemented, APIError{
			Error:     "invalid JSON: " + err.Error(),
			Timestamp: TimeNow().UTC().Format(time.RFC3339),
		})
		return
	}

	//
	writeJSON(w, http.StatusNotImplemented, APIError{
		Error:     "stop not implemented yet",
		Timestamp: TimeNow().UTC().Format(time.RFC3339),
	})
}

// Basic middleware: sets JSON content type and very lightweight logging.
// No CORS or auth because this is a local control-plane service.
func withBasicMiddleware(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := TimeNow()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
		dur := time.Since(start)
		logger.Printf("%s %s %dms UA=%q", r.Method, r.URL.Path, dur.Milliseconds(), r.UserAgent())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
