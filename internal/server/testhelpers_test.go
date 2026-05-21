package server

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
	"github.com/prometheus/client_golang/prometheus"
)

// fakeSender is a graph.Sender test double that records calls.
type fakeSender struct {
	mu    sync.Mutex
	calls []graph.Outgoing
	err   error
}

func (f *fakeSender) SendMail(_ context.Context, msg graph.Outgoing) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, msg)
	return f.err
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeSender) recorded() []graph.Outgoing {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]graph.Outgoing(nil), f.calls...)
}

// newTestServer builds a Server with sensible test defaults.
func newTestServer(t *testing.T, opts Options) *Server {
	t.Helper()
	if opts.Registry == nil {
		opts.Registry = prometheus.NewRegistry()
	}
	if opts.Renderer == nil {
		r, err := mail.NewRenderer("[Test]", mail.TemplateModern)
		if err != nil {
			t.Fatalf("NewRenderer() error: %v", err)
		}
		opts.Renderer = r
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return New(opts)
}

const sampleWebhook = `{
  "version": "4",
  "groupKey": "test-group",
  "status": "firing",
  "receiver": "graph-bridge",
  "externalURL": "http://am.example.com",
  "alerts": [
    {
      "status": "firing",
      "labels": { "alertname": "TestAlert", "severity": "warning" },
      "annotations": { "summary": "test summary" },
      "startsAt": "2026-05-20T08:00:00Z",
      "endsAt": "0001-01-01T00:00:00Z"
    }
  ]
}`

const emptyWebhook = `{"version":"4","groupKey":"g","status":"resolved","alerts":[]}`

const splitWebhook = `{
  "version": "4",
  "groupKey": "split-group",
  "status": "firing",
  "alerts": [
    { "status": "firing", "labels": { "alertname": "A", "email_to": "team-a@example.com" }, "annotations": {} },
    { "status": "firing", "labels": { "alertname": "B" }, "annotations": {} }
  ]
}`
