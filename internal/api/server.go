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
