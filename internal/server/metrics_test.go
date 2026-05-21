package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
)

func TestHealthz(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestReadyz(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})
	handler := srv.Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("before SetReady: status = %d, want 503", rec.Code)
	}

	srv.SetReady(true)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("after SetReady(true): status = %d, want 200", rec.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer(t, Options{
		Cfg:       config.ServerConfig{},
		Sender:    &fakeSender{},
		DefaultTo: []string{"ops@example.com"},
	})
	handler := srv.Handler()

	// Drive one webhook so a labelled counter is populated.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(sampleWebhook)))

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	bodyStr := string(body)
	for _, want := range []string{
		"agb_webhook_requests_total",
		"agb_webhook_request_duration_seconds",
		"agb_mails_sent_total",
		"agb_mail_send_duration_seconds",
		"agb_panics_recovered_total",
	} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("/metrics output missing %q", want)
		}
	}
	// The single webhook request must have been timed exactly once.
	if !strings.Contains(bodyStr, "agb_webhook_request_duration_seconds_count 1") {
		t.Errorf("/metrics missing the webhook duration observation (want count 1)")
	}
}
