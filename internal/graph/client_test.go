package graph

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer returns an httptest.Server that issues a static OAuth2 token.
func tokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":   "Bearer",
			"expires_in":   3599,
			"access_token": "test-access-token",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(t *testing.T, tokenURL, graphURL string, maxRetries int) *Client {
	t.Helper()
	c, err := New(Options{
		TenantID:     "tenant",
		ClientID:     "cid",
		ClientSecret: "secret",
		TokenURL:     tokenURL,
		GraphBaseURL: graphURL,
		From:         "sender@example.com",
		MaxRetries:   maxRetries,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return c
}

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "missing tenant", opts: Options{ClientID: "c", ClientSecret: "s"}},
		{name: "missing clientID", opts: Options{TenantID: "t", ClientSecret: "s"}},
		{name: "missing secret", opts: Options{TenantID: "t", ClientID: "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Error("New() error = nil, want validation error")
			}
		})
	}
}

func TestSendMailSuccess(t *testing.T) {
	tok := tokenServer(t)

	var (
		gotAuth string
		gotPath string
		gotBody []byte
	)
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	err := client.SendMail(context.Background(), Outgoing{
		To:              []string{"to@example.com"},
		Cc:              []string{"cc@example.com"},
		Subject:         "Test Subject",
		HTMLBody:        "<p>hello</p>",
		SaveToSentItems: true,
	})
	if err != nil {
		t.Fatalf("SendMail() error: %v", err)
	}

	if gotAuth != "Bearer test-access-token" {
		t.Errorf("Authorization = %q, want Bearer test-access-token", gotAuth)
	}
	if gotPath != "/users/sender@example.com/sendMail" {
		t.Errorf("path = %q, want /users/sender@example.com/sendMail", gotPath)
	}

	var env sendMailEnvelope
	if err := json.Unmarshal(gotBody, &env); err != nil {
		t.Fatalf("decoding request body: %v", err)
	}
	if env.Message.Subject != "Test Subject" {
		t.Errorf("Subject = %q", env.Message.Subject)
	}
	if env.Message.Body.ContentType != "HTML" {
		t.Errorf("ContentType = %q, want HTML", env.Message.Body.ContentType)
	}
	if !env.SaveToSentItems {
		t.Error("SaveToSentItems = false, want true")
	}
	if len(env.Message.ToRecipients) != 1 || env.Message.ToRecipients[0].EmailAddress.Address != "to@example.com" {
		t.Errorf("ToRecipients = %+v", env.Message.ToRecipients)
	}
	if len(env.Message.CcRecipients) != 1 {
		t.Errorf("CcRecipients = %+v, want 1 entry", env.Message.CcRecipients)
	}
}

func TestSendMailGraphClientError(t *testing.T) {
	tok := tokenServer(t)
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"ErrorInvalidRecipients","message":"bad recipients"}}`))
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	err := client.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v), want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Code != "ErrorInvalidRecipients" {
		t.Errorf("Code = %q, want ErrorInvalidRecipients", apiErr.Code)
	}
}

func TestSendMailServerError(t *testing.T) {
	tok := tokenServer(t)
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"InternalServerError","message":"boom"}}`))
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	err := client.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}})

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want *APIError with status 500", err)
	}
}

func TestSendMail429RetriesThenSucceeds(t *testing.T) {
	tok := tokenServer(t)
	var calls atomic.Int64
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"TooManyRequests","message":"slow down"}}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	if err := client.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}}); err != nil {
		t.Fatalf("SendMail() error: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("graph calls = %d, want 2 (initial + 1 retry)", got)
	}
}

func TestSendMail429Exhausted(t *testing.T) {
	tok := tokenServer(t)
	var calls atomic.Int64
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"TooManyRequests","message":"slow down"}}`))
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	err := client.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}})

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("error = %v, want *APIError with status 429", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("graph calls = %d, want 2 (initial + 1 retry)", got)
	}
}

func TestSendMail503RetriesThenSucceeds(t *testing.T) {
	tok := tokenServer(t)
	var calls atomic.Int64
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"ServiceUnavailable","message":"temporary"}}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	if err := client.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}}); err != nil {
		t.Fatalf("SendMail() error: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("graph calls = %d, want 2 (transient 503 should be retried)", got)
	}
}

func TestSendMailTokenFailure(t *testing.T) {
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret"}`))
	}))
	t.Cleanup(tok.Close)

	var graphCalls atomic.Int64
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		graphCalls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	err := client.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}})
	if err == nil {
		t.Fatal("SendMail() error = nil, want token failure")
	}
	if got := graphCalls.Load(); got != 0 {
		t.Errorf("graph calls = %d, want 0 (token must fail first)", got)
	}
}

func TestSendMailContextCancelled(t *testing.T) {
	tok := tokenServer(t)
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.SendMail(ctx, Outgoing{To: []string{"x@example.com"}})
	if err == nil {
		t.Fatal("SendMail() error = nil, want context cancellation error")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Errorf("error = %v, want a transport error not *APIError", err)
	}
}

func TestSendMailContextCancelledDuringRetry(t *testing.T) {
	tok := tokenServer(t)
	var calls atomic.Int64
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		// A long Retry-After forces SendMail to wait before retrying.
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"TooManyRequests","message":"slow down"}}`))
	}))
	t.Cleanup(graphSrv.Close)

	client := newTestClient(t, tok.URL, graphSrv.URL, 3)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := client.SendMail(ctx, Outgoing{To: []string{"x@example.com"}})
	if err == nil {
		t.Fatal("SendMail() error = nil, want context cancellation while waiting to retry")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("graph calls = %d, want 1 (cancelled before the retry fired)", got)
	}
}

func TestSendMailNoSender(t *testing.T) {
	tok := tokenServer(t)
	c, err := New(Options{
		TenantID: "t", ClientID: "c", ClientSecret: "s", TokenURL: tok.URL,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if err := c.SendMail(context.Background(), Outgoing{To: []string{"x@example.com"}}); err == nil {
		t.Error("SendMail() error = nil, want missing-sender error")
	}
}
