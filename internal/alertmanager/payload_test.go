package alertmanager

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name           string
		fixture        string
		body           string
		wantErr        error
		wantGenericErr bool
		wantAlerts     int
		wantVersion    string
	}{
		{name: "firing payload", fixture: "firing.json", wantAlerts: 2, wantVersion: "4"},
		{name: "resolved payload", fixture: "resolved.json", wantAlerts: 1, wantVersion: "4"},
		{name: "mixed payload", fixture: "mixed.json", wantAlerts: 2, wantVersion: "4"},
		{name: "empty alerts", fixture: "empty.json", wantErr: ErrEmptyPayload},
		{
			name:    "unsupported version",
			body:    `{"version":"3","alerts":[{"status":"firing"}]}`,
			wantErr: ErrUnsupportedVersion,
		},
		{
			name:           "malformed json",
			body:           `{"version":"4","alerts":[`,
			wantGenericErr: true,
		},
		{
			name:           "not json at all",
			body:           `this is not json`,
			wantGenericErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r io.Reader
			if tt.fixture != "" {
				data, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
				if err != nil {
					t.Fatalf("reading fixture: %v", err)
				}
				r = bytes.NewReader(data)
			} else {
				r = strings.NewReader(tt.body)
			}

			p, err := Parse(r)

			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse() error = %v, want %v", err, tt.wantErr)
				}
				return
			case tt.wantGenericErr:
				if err == nil {
					t.Fatal("Parse() error = nil, want a decoding error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if len(p.Alerts) != tt.wantAlerts {
				t.Errorf("len(Alerts) = %d, want %d", len(p.Alerts), tt.wantAlerts)
			}
			if p.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", p.Version, tt.wantVersion)
			}
		})
	}
}

func TestParseFieldMapping(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "resolved.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	p, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if p.GroupKey != `{}:{alertname="DiskSpaceLow"}` {
		t.Errorf("GroupKey = %q", p.GroupKey)
	}
	if p.ExternalURL != "http://alertmanager.example.com" {
		t.Errorf("ExternalURL = %q", p.ExternalURL)
	}

	a := p.Alerts[0]
	if a.Fingerprint != "fff000aaa111" {
		t.Errorf("Fingerprint = %q", a.Fingerprint)
	}
	if a.StartsAt.IsZero() {
		t.Error("StartsAt should be parsed")
	}
	if a.EndsAt.IsZero() {
		t.Error("EndsAt should be set for a resolved alert")
	}
	if a.GeneratorURL == "" {
		t.Error("GeneratorURL should be parsed")
	}
}

func TestAlertEmailTo(t *testing.T) {
	tests := []struct {
		name     string
		hasLabel bool
		label    string
		want     []string
	}{
		{name: "label absent", hasLabel: false, want: nil},
		{name: "single address", hasLabel: true, label: "ops@example.com", want: []string{"ops@example.com"}},
		{
			name:     "multiple with whitespace",
			hasLabel: true,
			label:    " a@example.com , b@example.com ",
			want:     []string{"a@example.com", "b@example.com"},
		},
		{name: "trailing comma", hasLabel: true, label: "a@example.com,", want: []string{"a@example.com"}},
		{name: "only separators", hasLabel: true, label: ", ,", want: nil},
		{name: "empty value", hasLabel: true, label: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := map[string]string{}
			if tt.hasLabel {
				labels["email_to"] = tt.label
			}
			got := Alert{Labels: labels}.EmailTo()
			if !slices.Equal(got, tt.want) {
				t.Errorf("EmailTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

// FuzzParse ensures Parse never panics on arbitrary, untrusted input.
func FuzzParse(f *testing.F) {
	seeds := []string{
		`{"version":"4","alerts":[{"status":"firing","labels":{"alertname":"X"}}]}`,
		`{"version":"4","alerts":[]}`,
		`{"version":"3","alerts":[{"status":"firing"}]}`,
		`{"version":"4","alerts":[{"startsAt":"not-a-timestamp"}]}`,
		`{"version":"4"}`,
		``,
		`not json at all`,
		`{`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data string) {
		p, err := Parse(strings.NewReader(data))
		if err == nil && p == nil {
			t.Error("Parse returned a nil payload with a nil error")
		}
		if err == nil && len(p.Alerts) == 0 {
			t.Error("Parse succeeded but produced no alerts")
		}
	})
}

func TestAlertAccessors(t *testing.T) {
	firing := Alert{
		Status:      "firing",
		Labels:      map[string]string{"alertname": "HighCPU", "severity": "critical"},
		Annotations: map[string]string{"summary": "cpu hot"},
	}
	if got := firing.Name(); got != "HighCPU" {
		t.Errorf("Name() = %q, want HighCPU", got)
	}
	if got := firing.Severity(); got != "critical" {
		t.Errorf("Severity() = %q, want critical", got)
	}
	if got := firing.Summary(); got != "cpu hot" {
		t.Errorf("Summary() = %q, want cpu hot", got)
	}
	if !firing.IsFiring() {
		t.Error("IsFiring() = false, want true")
	}

	bare := Alert{Status: "resolved"}
	if got := bare.Name(); got != "unknown" {
		t.Errorf("Name() = %q, want unknown", got)
	}
	if got := bare.Severity(); got != "unknown" {
		t.Errorf("Severity() = %q, want unknown", got)
	}
	if bare.IsFiring() {
		t.Error("IsFiring() = true, want false")
	}

	descOnly := Alert{Annotations: map[string]string{"description": "fallback text"}}
	if got := descOnly.Summary(); got != "fallback text" {
		t.Errorf("Summary() = %q, want fallback text", got)
	}
}
