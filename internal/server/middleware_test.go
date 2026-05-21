package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		configToken string
		header      string
		wantStatus  int
	}{
		{name: "no auth configured", configToken: "", header: "", wantStatus: http.StatusOK},
		{name: "correct token", configToken: "s3cret", header: "Bearer s3cret", wantStatus: http.StatusOK},
		{name: "wrong token", configToken: "s3cret", header: "Bearer nope", wantStatus: http.StatusUnauthorized},
		{name: "missing header", configToken: "s3cret", header: "", wantStatus: http.StatusUnauthorized},
		{name: "missing bearer prefix", configToken: "s3cret", header: "s3cret", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, Options{
				Cfg:    config.ServerConfig{BearerToken: tt.configToken},
				Sender: &fakeSender{},
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(sampleWebhook))
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRecoverMiddleware(t *testing.T) {
	srv := newTestServer(t, Options{Cfg: config.ServerConfig{}, Sender: &fakeSender{}})

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler exploded")
	})
	rec := httptest.NewRecorder()
	srv.recoverMiddleware(panicking).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if got := testutil.ToFloat64(srv.metrics.PanicsRecovered); got != 1 {
		t.Errorf("PanicsRecovered = %v, want 1", got)
	}
}
