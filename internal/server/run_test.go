package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
)

func TestRunGracefulShutdown(t *testing.T) {
	srv := newTestServer(t, Options{
		Cfg:    config.ServerConfig{Port: 0, ShutdownGrace: config.Duration(2 * time.Second)},
		Sender: &fakeSender{},
	})
	srv.SetReady(true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// Allow the listener to start before requesting shutdown.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	// Occupy a port so the server cannot bind it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	srv := newTestServer(t, Options{
		Cfg:    config.ServerConfig{Port: port, ShutdownGrace: config.Duration(time.Second)},
		Sender: &fakeSender{},
	})
	if err := srv.Run(context.Background()); err == nil {
		t.Error("Run() error = nil, want a listen error for the occupied port")
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	srv := New(Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})

	if srv.logger == nil {
		t.Error("logger should default to a non-nil logger")
	}
	if srv.registry == nil {
		t.Error("registry should be created when not provided")
	}
	if srv.sendTimeout != defaultSendTimeout {
		t.Errorf("sendTimeout = %v, want default %v", srv.sendTimeout, defaultSendTimeout)
	}
	if srv.Handler() == nil {
		t.Error("Handler() returned nil")
	}
}

// failingResponseWriter fails every Write, used to exercise writeJSON's
// encode-error branch.
type failingResponseWriter struct {
	header http.Header
}

func (f *failingResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}

func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (f *failingResponseWriter) WriteHeader(int) {}

func TestWriteJSONHandlesWriteError(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})
	// Must not panic even though the underlying writer fails.
	srv.writeJSON(&failingResponseWriter{}, http.StatusOK, "processed")
}
