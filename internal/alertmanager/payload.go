// Package alertmanager defines the Prometheus Alertmanager webhook payload
// (schema version 4) and decodes it from a request body.
package alertmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// WebhookVersion is the only Alertmanager webhook schema version supported.
const WebhookVersion = "4"

// Errors returned by Parse.
var (
	ErrEmptyPayload       = errors.New("alertmanager: payload contains no alerts")
	ErrUnsupportedVersion = errors.New("alertmanager: unsupported webhook version")
)

// Payload is an Alertmanager webhook notification.
type Payload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

// Alert is a single alert within a webhook payload.
type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// Parse decodes and validates an Alertmanager webhook body.
//
// A malformed body yields a JSON decoding error. A well-formed body with an
// unexpected version yields ErrUnsupportedVersion, and one with no alerts
// yields ErrEmptyPayload.
func Parse(r io.Reader) (*Payload, error) {
	var p Payload
	dec := json.NewDecoder(r)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("alertmanager: decoding payload: %w", err)
	}
	if p.Version != WebhookVersion {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrUnsupportedVersion, p.Version, WebhookVersion)
	}
	if len(p.Alerts) == 0 {
		return nil, ErrEmptyPayload
	}
	return &p, nil
}

// Name returns the alert name from the alertname label.
func (a Alert) Name() string {
	if n := a.Labels["alertname"]; n != "" {
		return n
	}
	return "unknown"
}

// Summary returns the summary annotation, falling back to description.
func (a Alert) Summary() string {
	if s := a.Annotations["summary"]; s != "" {
		return s
	}
	return a.Annotations["description"]
}

// Severity returns the severity label, or "unknown" if absent.
func (a Alert) Severity() string {
	if s := a.Labels["severity"]; s != "" {
		return s
	}
	return "unknown"
}

// IsFiring reports whether the alert is currently firing.
func (a Alert) IsFiring() bool {
	return a.Status == "firing"
}

// EmailTo returns the recipient addresses from the email_to label, split on
// commas. It returns nil when the label is absent or empty.
func (a Alert) EmailTo() []string {
	raw, ok := a.Labels["email_to"]
	if !ok {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
