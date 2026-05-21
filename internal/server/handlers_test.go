package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHandleAlerts(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		senderErr  error
		wantStatus int
		wantSends  int
	}{
		{name: "valid payload", body: sampleWebhook, wantStatus: http.StatusOK, wantSends: 1},
		{name: "empty alerts", body: emptyWebhook, wantStatus: http.StatusOK, wantSends: 0},
		{name: "invalid json", body: `{"version":"4","alerts":[`, wantStatus: http.StatusBadRequest, wantSends: 0},
		{
			name:       "unsupported version",
			body:       `{"version":"3","alerts":[{"status":"firing"}]}`,
			wantStatus: http.StatusBadRequest,
			wantSends:  0,
		},
		{
			name:       "sender failure",
			body:       sampleWebhook,
			senderErr:  errors.New("smtp exploded"),
			wantStatus: http.StatusBadGateway,
			wantSends:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSender{err: tt.senderErr}
			srv := newTestServer(t, Options{
				Cfg:       config.ServerConfig{},
				Sender:    sender,
				DefaultTo: []string{"ops@example.com"},
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if sender.count() != tt.wantSends {
				t.Errorf("sender calls = %d, want %d", sender.count(), tt.wantSends)
			}
		})
	}
}

func TestHandleAlertsSplitsRecipients(t *testing.T) {
	sender := &fakeSender{}
	srv := newTestServer(t, Options{
		Cfg:       config.ServerConfig{},
		Sender:    sender,
		DefaultTo: []string{"default@example.com"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(splitWebhook))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if sender.count() != 2 {
		t.Fatalf("sender calls = %d, want 2 (one per recipient set)", sender.count())
	}

	recipients := map[string]bool{}
	for _, msg := range sender.recorded() {
		recipients[strings.Join(msg.To, ",")] = true
	}
	if !recipients["team-a@example.com"] || !recipients["default@example.com"] {
		t.Errorf("recipient sets = %v, want team-a and default groups", recipients)
	}
}

func TestHandleAlertsRecordsErrorReason(t *testing.T) {
	sender := &fakeSender{err: &graph.APIError{StatusCode: http.StatusTooManyRequests}}
	srv := newTestServer(t, Options{
		Cfg:       config.ServerConfig{},
		Sender:    sender,
		DefaultTo: []string{"ops@example.com"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(sampleWebhook))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if got := testutil.ToFloat64(srv.metrics.MailSendErrors.WithLabelValues("graph_429")); got != 1 {
		t.Errorf("graph_429 error count = %v, want 1", got)
	}
}

func TestHandleAlertsConcurrent(t *testing.T) {
	sender := &fakeSender{}
	srv := newTestServer(t, Options{
		Cfg:       config.ServerConfig{},
		Sender:    sender,
		DefaultTo: []string{"ops@example.com"},
	})
	handler := srv.Handler()

	const n = 25
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(
				http.MethodPost, "/api/v1/alerts", strings.NewReader(sampleWebhook)))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		}()
	}
	wg.Wait()

	if sender.count() != n {
		t.Errorf("sender calls = %d, want %d", sender.count(), n)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "429", err: &graph.APIError{StatusCode: 429}, want: "graph_429"},
		{name: "500", err: &graph.APIError{StatusCode: 503}, want: "graph_5xx"},
		{name: "400", err: &graph.APIError{StatusCode: 400}, want: "graph_4xx"},
		{name: "transport", err: errors.New("dial failed"), want: "transport"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyError(tt.err); got != tt.want {
				t.Errorf("classifyError() = %q, want %q", got, tt.want)
			}
		})
	}
}
