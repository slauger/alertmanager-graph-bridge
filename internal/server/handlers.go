package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/alertmanager"
	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
)

// maxBodyBytes bounds the accepted webhook request body size.
const maxBodyBytes = 5 << 20

// handleAlerts receives an Alertmanager webhook, groups the alerts by recipient
// set and delivers each group as an e-mail.
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() { s.metrics.WebhookDuration.Observe(time.Since(start).Seconds()) }()

	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)

	payload, err := alertmanager.Parse(body)
	if err != nil {
		switch {
		case errors.Is(err, alertmanager.ErrEmptyPayload):
			s.metrics.WebhookRequests.WithLabelValues("empty").Inc()
			s.writeJSON(w, http.StatusOK, "no alerts to process")
		case errors.Is(err, alertmanager.ErrUnsupportedVersion):
			s.metrics.WebhookRequests.WithLabelValues("bad_request").Inc()
			s.logger.Warn("rejected webhook", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			s.metrics.WebhookRequests.WithLabelValues("bad_request").Inc()
			s.logger.Warn("failed to parse webhook", "error", err)
			http.Error(w, "invalid payload", http.StatusBadRequest)
		}
		return
	}

	groups := mail.GroupAlerts(payload, s.defaultTo)
	ctx, cancel := context.WithTimeout(r.Context(), s.sendTimeout)
	defer cancel()

	var failed int
	for _, g := range groups {
		if derr := s.deliver(ctx, g); derr != nil {
			failed++
			s.logger.Error("failed to deliver alert group",
				"groupKey", g.GroupKey, "recipients", g.Recipients, "error", derr)
			continue
		}
		s.logger.Info("delivered alert group",
			"groupKey", g.GroupKey, "recipients", g.Recipients, "alerts", len(g.Alerts))
	}

	if failed > 0 {
		s.metrics.WebhookRequests.WithLabelValues("send_failed").Inc()
		http.Error(w, "failed to deliver one or more alert groups", http.StatusBadGateway)
		return
	}
	s.metrics.WebhookRequests.WithLabelValues("ok").Inc()
	s.writeJSON(w, http.StatusOK, "alerts processed")
}

// deliver renders and sends a single alert group, recording metrics.
func (s *Server) deliver(ctx context.Context, g mail.Group) error {
	msg, err := s.renderer.Render(g)
	if err != nil {
		s.metrics.MailSendErrors.WithLabelValues("render").Inc()
		return fmt.Errorf("render alert group: %w", err)
	}

	start := time.Now()
	err = s.sender.SendMail(ctx, graph.Outgoing{
		To:              msg.To,
		Subject:         msg.Subject,
		HTMLBody:        msg.HTMLBody,
		SaveToSentItems: s.saveToSent,
	})
	s.metrics.SendDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		s.metrics.MailSendErrors.WithLabelValues(classifyError(err)).Inc()
		return err
	}
	s.metrics.MailsSent.Inc()
	return nil
}

// classifyError maps a send error to a metric reason label.
func classifyError(err error) string {
	var apiErr *graph.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusTooManyRequests:
			return "graph_429"
		case apiErr.StatusCode >= 500:
			return "graph_5xx"
		case apiErr.StatusCode >= 400:
			return "graph_4xx"
		}
	}
	return "transport"
}
