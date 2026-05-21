// Package server wires the HTTP API: the Alertmanager webhook receiver,
// health/readiness probes and the Prometheus metrics endpoint.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	defaultSendTimeout = 20 * time.Second
	readHeaderTimeout  = 5 * time.Second
	idleTimeout        = 60 * time.Second
	maxHeaderBytes     = 1 << 20
)

// Server is the HTTP application.
type Server struct {
	cfg         config.ServerConfig
	sender      graph.Sender
	renderer    *mail.Renderer
	metrics     *Metrics
	logger      *slog.Logger
	registry    *prometheus.Registry
	defaultTo   []string
	sendTimeout time.Duration
	saveToSent  bool
	ready       atomic.Bool
}

// Options configures a Server.
type Options struct {
	Cfg             config.ServerConfig
	Sender          graph.Sender
	Renderer        *mail.Renderer
	DefaultTo       []string
	Logger          *slog.Logger
	Registry        *prometheus.Registry
	SendTimeout     time.Duration
	SaveToSentItems bool
}

// New constructs a Server.
func New(opts Options) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	registry := opts.Registry
	if registry == nil {
		registry = prometheus.NewRegistry()
	}
	sendTimeout := opts.SendTimeout
	if sendTimeout <= 0 {
		sendTimeout = defaultSendTimeout
	}
	return &Server{
		cfg:         opts.Cfg,
		sender:      opts.Sender,
		renderer:    opts.Renderer,
		metrics:     NewMetrics(registry),
		logger:      logger,
		registry:    registry,
		defaultTo:   opts.DefaultTo,
		sendTimeout: sendTimeout,
		saveToSent:  opts.SaveToSentItems,
	}
}

// Handler returns the fully routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/alerts", s.authMiddleware(http.HandlerFunc(s.handleAlerts)))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	return s.recoverMiddleware(mux)
}

// SetReady toggles the readiness state reported by /readyz.
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts it
// down gracefully within the configured grace period.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(s.cfg.Port)),
		Handler:           s.Handler(),
		ReadTimeout:       s.cfg.ReadTimeout.Std(),
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      s.cfg.WriteTimeout.Std(),
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownGrace.Std())
	defer cancel()
	s.logger.Info("shutting down http server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if !s.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// writeJSON writes a small JSON status response.
func (s *Server) writeJSON(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": status}); err != nil {
		s.logger.Warn("failed to write JSON response", "error", err)
	}
}
