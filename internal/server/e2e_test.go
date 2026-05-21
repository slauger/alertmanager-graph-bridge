package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// recorder captures Graph sendMail request bodies in a race-safe way.
type recorder struct {
	mu     sync.Mutex
	hits   int
	bodies [][]byte
}

func (r *recorder) record(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits++
	r.bodies = append(r.bodies, b)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits
}

func (r *recorder) snapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.bodies...)
}

// okTokenHandler issues a static OAuth2 token.
func okTokenHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token_type":   "Bearer",
		"expires_in":   3599,
		"access_token": "e2e-token",
	})
}

// wireBridge starts a mocked OAuth2 token server, a mocked Graph server and the
// fully wired bridge HTTP server. It returns the bridge base URL and the Server
// so tests can inspect metrics.
func wireBridge(t *testing.T, cfg config.ServerConfig, tokenHandler, graphHandler http.HandlerFunc) (string, *Server) {
	t.Helper()

	tokenSrv := httptest.NewServer(tokenHandler)
	t.Cleanup(tokenSrv.Close)

	graphSrv := httptest.NewServer(graphHandler)
	t.Cleanup(graphSrv.Close)

	gClient, err := graph.New(graph.Options{
		TenantID:     "tenant",
		ClientID:     "cid",
		ClientSecret: "secret",
		TokenURL:     tokenSrv.URL,
		GraphBaseURL: graphSrv.URL,
		From:         "monitoring@example.com",
		MaxRetries:   1,
	})
	if err != nil {
		t.Fatalf("graph.New() error: %v", err)
	}

	renderer, err := mail.NewRenderer("[E2E]", mail.TemplateModern)
	if err != nil {
		t.Fatalf("mail.NewRenderer() error: %v", err)
	}

	srv := newTestServer(t, Options{
		Cfg:       cfg,
		Sender:    gClient,
		Renderer:  renderer,
		DefaultTo: []string{"ops@example.com"},
	})
	bridge := httptest.NewServer(srv.Handler())
	t.Cleanup(bridge.Close)
	return bridge.URL, srv
}

func postWebhook(t *testing.T, baseURL, body, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/alerts", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST webhook: %v", err)
	}
	return resp
}

func TestEndToEndDelivery(t *testing.T) {
	rec := &recorder{}
	url, _ := wireBridge(t, config.ServerConfig{}, okTokenHandler, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.record(b)
		w.WriteHeader(http.StatusAccepted)
	})

	resp := postWebhook(t, url, sampleWebhook, "")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if rec.count() != 1 {
		t.Fatalf("graph requests = %d, want 1", rec.count())
	}

	body := string(rec.snapshot()[0])
	if !strings.Contains(body, "TestAlert") {
		t.Errorf("graph request body missing alert name: %s", body)
	}
	if !strings.Contains(body, `"contentType":"HTML"`) {
		t.Errorf("graph request body should carry HTML content type: %s", body)
	}
}

func TestEndToEndSplitsRecipients(t *testing.T) {
	rec := &recorder{}
	url, _ := wireBridge(t, config.ServerConfig{}, okTokenHandler, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.record(b)
		w.WriteHeader(http.StatusAccepted)
	})

	resp := postWebhook(t, url, splitWebhook, "")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if rec.count() != 2 {
		t.Fatalf("graph requests = %d, want 2 (one per recipient set)", rec.count())
	}

	seen := map[string]bool{}
	for _, raw := range rec.snapshot() {
		var env struct {
			Message struct {
				ToRecipients []struct {
					EmailAddress struct {
						Address string `json:"address"`
					} `json:"emailAddress"`
				} `json:"toRecipients"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decoding graph request: %v", err)
		}
		var addrs []string
		for _, r := range env.Message.ToRecipients {
			addrs = append(addrs, r.EmailAddress.Address)
		}
		seen[strings.Join(addrs, ",")] = true
	}
	if !seen["team-a@example.com"] || !seen["ops@example.com"] {
		t.Errorf("recipient sets = %v, want team-a and ops groups", seen)
	}
}

func TestEndToEndRetryOn429(t *testing.T) {
	var hits atomic.Int64
	url, _ := wireBridge(t, config.ServerConfig{}, okTokenHandler, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"TooManyRequests","message":"slow down"}}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	resp := postWebhook(t, url, sampleWebhook, "")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after retry", resp.StatusCode)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("graph requests = %d, want 2 (initial + retry)", got)
	}
}

func TestEndToEndBearerAuth(t *testing.T) {
	var graphHits atomic.Int64
	cfg := config.ServerConfig{BearerToken: "e2e-secret"}
	url, _ := wireBridge(t, cfg, okTokenHandler, func(w http.ResponseWriter, _ *http.Request) {
		graphHits.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})

	// Wrong token: rejected, Graph never called.
	bad := postWebhook(t, url, sampleWebhook, "wrong")
	_ = bad.Body.Close()
	if bad.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", bad.StatusCode)
	}

	// No token at all.
	missing := postWebhook(t, url, sampleWebhook, "")
	_ = missing.Body.Close()
	if missing.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing token: status = %d, want 401", missing.StatusCode)
	}

	if graphHits.Load() != 0 {
		t.Errorf("graph called %d times for unauthorized requests, want 0", graphHits.Load())
	}

	// Correct token: accepted and delivered.
	ok := postWebhook(t, url, sampleWebhook, "e2e-secret")
	_ = ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Errorf("correct token: status = %d, want 200", ok.StatusCode)
	}
	if graphHits.Load() != 1 {
		t.Errorf("graph called %d times, want 1", graphHits.Load())
	}
}

func TestEndToEndGraphErrorReturns502(t *testing.T) {
	url, srv := wireBridge(t, config.ServerConfig{}, okTokenHandler, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"ErrorInvalidRecipients","message":"bad"}}`))
	})

	resp := postWebhook(t, url, sampleWebhook, "")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if got := testutil.ToFloat64(srv.metrics.MailSendErrors.WithLabelValues("graph_4xx")); got != 1 {
		t.Errorf("graph_4xx error count = %v, want 1", got)
	}
}

func TestEndToEndTokenFailure(t *testing.T) {
	var graphHits atomic.Int64
	tokenFails := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret"}`))
	}
	url, _ := wireBridge(t, config.ServerConfig{}, tokenFails, func(w http.ResponseWriter, _ *http.Request) {
		graphHits.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})

	resp := postWebhook(t, url, sampleWebhook, "")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when the token cannot be obtained", resp.StatusCode)
	}
	if graphHits.Load() != 0 {
		t.Errorf("graph called %d times, want 0 (token must fail first)", graphHits.Load())
	}
}

func TestEndToEndMetrics(t *testing.T) {
	url, srv := wireBridge(t, config.ServerConfig{}, okTokenHandler, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	resp := postWebhook(t, url, sampleWebhook, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if got := testutil.ToFloat64(srv.metrics.MailsSent); got != 1 {
		t.Errorf("agb_mails_sent_total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(srv.metrics.WebhookRequests.WithLabelValues("ok")); got != 1 {
		t.Errorf("agb_webhook_requests_total{outcome=ok} = %v, want 1", got)
	}

	// The /metrics endpoint exposes the populated counters.
	metricsResp, err := http.Get(url + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = metricsResp.Body.Close() }()
	body, _ := io.ReadAll(metricsResp.Body)
	if !strings.Contains(string(body), "agb_mails_sent_total 1") {
		t.Errorf("/metrics output missing populated agb_mails_sent_total")
	}
}
