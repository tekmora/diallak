// Package health provides an optional HTTP listener for liveness and readiness probes.
package health

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	// ReadHeaderTimeout is the maximum duration for reading request headers.
	// Chosen, not measured.
	ReadHeaderTimeout = 2 * time.Second

	// ReadTimeout is the maximum duration for reading the entire request.
	// Chosen, not measured.
	ReadTimeout = 5 * time.Second

	// WriteTimeout is the maximum duration for writing the response.
	// Chosen, not measured.
	WriteTimeout = 5 * time.Second

	// IdleTimeout is the maximum duration an idle keep-alive connection remains open.
	// Chosen, not measured.
	IdleTimeout = 30 * time.Second

	// MaxHeaderBytes limits the maximum size of request headers.
	// Chosen, not measured.
	MaxHeaderBytes = 4096

	// ShutdownDeadline is the maximum duration to wait for active requests during graceful shutdown.
	// Chosen, not measured.
	ShutdownDeadline = 5 * time.Second
)

// State holds the atomic readiness state of the application.
// The zero value represents not ready.
type State struct {
	ready atomic.Bool
}

// Ready reports whether the application is ready to serve.
func (s *State) Ready() bool {
	if s == nil {
		return false
	}
	return s.ready.Load()
}

// SetReady sets whether the application is ready to serve.
func (s *State) SetReady(ready bool) {
	if s == nil {
		return
	}
	s.ready.Store(ready)
}

// Handler serves HTTP liveness (/healthz) and readiness (/readyz) probes.
type Handler struct {
	state *State
}

// NewHandler returns a new Handler reporting readiness from state.
func NewHandler(state *State) *Handler {
	return &Handler{state: state}
}

// ServeHTTP handles /healthz, /readyz, and returns 404 for any other path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	switch r.URL.Path {
	case "/healthz":
		h.handleHealthz(w, r)
	case "/readyz":
		h.handleReadyz(w, r)
	default:
		writeResponse(w, r, http.StatusNotFound, "not found\n")
	}
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeResponse(w, r, http.StatusMethodNotAllowed, "method not allowed\n")
		return
	}
	writeResponse(w, r, http.StatusOK, "ok\n")
}

func (h *Handler) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeResponse(w, r, http.StatusMethodNotAllowed, "method not allowed\n")
		return
	}
	if h.state != nil && h.state.Ready() {
		writeResponse(w, r, http.StatusOK, "ready\n")
		return
	}
	writeResponse(w, r, http.StatusServiceUnavailable, "not ready\n")
}

func writeResponse(w http.ResponseWriter, r *http.Request, statusCode int, body string) {
	w.WriteHeader(statusCode)
	if r.Method != http.MethodHead && body != "" {
		if _, writeErr := w.Write([]byte(body)); writeErr != nil {
			return
		}
	}
}

// Server serves HTTP health probes.
type Server struct {
	server           *http.Server
	shutdownDeadline time.Duration
}

type serverOption func(*Server)

func withReadHeaderTimeout(d time.Duration) serverOption {
	return func(s *Server) {
		s.server.ReadHeaderTimeout = d
	}
}

// NewServer constructs an HTTP probe server with standard timeouts.
//
// The handler writes no log line. What the HTTP server itself reports, such as a failing accept,
// goes to logger as a warning; a nil logger drops it. Without this the standard library would
// print such lines unstructured to standard error.
func NewServer(state *State, logger *slog.Logger, opts ...serverOption) *Server {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Server{
		server: &http.Server{
			Handler:           NewHandler(state),
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
			ReadHeaderTimeout: ReadHeaderTimeout,
			ReadTimeout:       ReadTimeout,
			WriteTimeout:      WriteTimeout,
			IdleTimeout:       IdleTimeout,
			MaxHeaderBytes:    MaxHeaderBytes,
		},
		shutdownDeadline: ShutdownDeadline,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Listen starts a TCP listener on addr bound to ctx.
func Listen(ctx context.Context, addr string) (net.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	return ln, nil
}

// Serve serves HTTP connections on ln until ctx is canceled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	var shutdownErr error
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownDeadline)
		defer cancel()
		shutdownErr = s.server.Shutdown(shutdownCtx)
		close(done)
	})
	defer stop()

	serveErr := s.server.Serve(ln)
	if ctx.Err() != nil {
		<-done
		if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
			return fmt.Errorf("shutdown health: %w", shutdownErr)
		}
		return nil
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve health: %w", serveErr)
	}
	return nil
}
