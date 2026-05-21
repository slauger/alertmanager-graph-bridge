package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
)

// TestHandleHealthz verifies the liveness probe is always served and reports OK.
// It also guards the route wiring in Handler().
func TestHandleHealthz(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// TestHandleReadyz exercises the readiness state machine: a fresh server is not
// ready (503), SetReady(true) flips it to ready (200) and SetReady(false) -- the
// path the graceful shutdown takes -- flips it back to 503.
func TestHandleReadyz(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})
	handler := srv.Handler()

	probe := func() (int, string) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rec.Code, rec.Body.String()
	}

	// A freshly constructed server has not been marked ready yet.
	if code, body := probe(); code != http.StatusServiceUnavailable || body != "not ready" {
		t.Errorf("before SetReady: status = %d body = %q, want 503 %q", code, body, "not ready")
	}

	srv.SetReady(true)
	if code, body := probe(); code != http.StatusOK || body != "ready" {
		t.Errorf("after SetReady(true): status = %d body = %q, want 200 %q", code, body, "ready")
	}

	// Graceful shutdown calls SetReady(false); /readyz must report 503 again so
	// Kubernetes drains the pod from the Service before it stops.
	srv.SetReady(false)
	if code, body := probe(); code != http.StatusServiceUnavailable || body != "not ready" {
		t.Errorf("after SetReady(false): status = %d body = %q, want 503 %q", code, body, "not ready")
	}
}

// TestHandleMetrics verifies the /metrics route is wired to the server's own
// registry and serves the bridge's registered metric families. Only the plain
// Counter and Histogram families are asserted: a CounterVec emits no series
// until a label combination is first observed, so agb_webhook_requests_total
// and agb_mail_send_errors_total are absent on an untouched server.
func TestHandleMetrics(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"agb_mails_sent_total",
		"agb_panics_recovered_total",
		"agb_webhook_request_duration_seconds",
		"agb_mail_send_duration_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics output missing %q metric family", want)
		}
	}
}
