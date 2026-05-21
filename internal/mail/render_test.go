package mail

import (
	"strings"
	"testing"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/alertmanager"
	"github.com/slauger/alertmanager-graph-bridge/internal/branding"
)

func mustRenderer(t *testing.T, prefix string) *Renderer {
	t.Helper()
	r, err := NewRenderer(prefix, TemplateModern)
	if err != nil {
		t.Fatalf("NewRenderer() error: %v", err)
	}
	return r
}

func TestRenderSubject(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		alerts []alertmanager.Alert
		want   string
	}{
		{
			name:   "single firing with summary",
			prefix: "[Mon]",
			alerts: []alertmanager.Alert{{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighCPU"},
				Annotations: map[string]string{"summary": "cpu hot"},
			}},
			want: "[Mon] [FIRING] HighCPU - cpu hot",
		},
		{
			name:   "single resolved without summary",
			prefix: "[Mon]",
			alerts: []alertmanager.Alert{{
				Status: "resolved",
				Labels: map[string]string{"alertname": "DiskLow"},
			}},
			want: "[Mon] [RESOLVED] DiskLow",
		},
		{
			name:   "multiple same alertname",
			prefix: "[Mon]",
			alerts: []alertmanager.Alert{
				{Status: "firing", Labels: map[string]string{"alertname": "X"}},
				{Status: "firing", Labels: map[string]string{"alertname": "X"}},
			},
			want: "[Mon] [FIRING] X (2 alerts)",
		},
		{
			name:   "multiple mixed status and names",
			prefix: "[Mon]",
			alerts: []alertmanager.Alert{
				{Status: "firing", Labels: map[string]string{"alertname": "X"}},
				{Status: "resolved", Labels: map[string]string{"alertname": "Y"}},
			},
			want: "[Mon] [FIRING/RESOLVED] 2 alerts",
		},
		{
			name:   "no prefix",
			prefix: "",
			alerts: []alertmanager.Alert{{
				Status: "firing",
				Labels: map[string]string{"alertname": "X"},
			}},
			want: "[FIRING] X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mustRenderer(t, tt.prefix)
			msg, err := r.Render(Group{Alerts: tt.alerts})
			if err != nil {
				t.Fatalf("Render() error: %v", err)
			}
			if msg.Subject != tt.want {
				t.Errorf("Subject = %q, want %q", msg.Subject, tt.want)
			}
		})
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	r := mustRenderer(t, "")
	msg, err := r.Render(Group{Alerts: []alertmanager.Alert{{
		Status:      "firing",
		Labels:      map[string]string{"alertname": "X"},
		Annotations: map[string]string{"summary": "<script>alert(1)</script>"},
	}}})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(msg.HTMLBody, "<script>alert(1)</script>") {
		t.Error("HTML body must not contain an unescaped script tag")
	}
	if !strings.Contains(msg.HTMLBody, "&lt;script&gt;") {
		t.Error("HTML body should contain the escaped script tag")
	}
}

func TestRenderEndsAtVisibility(t *testing.T) {
	r := mustRenderer(t, "")
	starts := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)

	firing, err := r.Render(Group{Alerts: []alertmanager.Alert{{
		Status:   "firing",
		Labels:   map[string]string{"alertname": "X"},
		StartsAt: starts,
	}}})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(firing.HTMLBody, "Ends at") {
		t.Error("firing alert must not render an Ends at row")
	}

	resolved, err := r.Render(Group{Alerts: []alertmanager.Alert{{
		Status:   "resolved",
		Labels:   map[string]string{"alertname": "X"},
		StartsAt: starts,
		EndsAt:   starts.Add(time.Hour),
	}}})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(resolved.HTMLBody, "Ends at") {
		t.Error("resolved alert must render an Ends at row")
	}
}

func TestRenderBodyContent(t *testing.T) {
	r := mustRenderer(t, "")
	msg, err := r.Render(Group{
		ExternalURL: "http://alertmanager.example.com",
		Recipients:  []string{"ops@example.com"},
		Alerts: []alertmanager.Alert{{
			Status:       "firing",
			Labels:       map[string]string{"alertname": "HighCPU", "severity": "warning"},
			Annotations:  map[string]string{"summary": "cpu hot"},
			GeneratorURL: "http://prometheus.example.com/graph",
		}},
	})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	for _, want := range []string{"HighCPU", "warning", "cpu hot", "http://prometheus.example.com/graph"} {
		if !strings.Contains(msg.HTMLBody, want) {
			t.Errorf("HTML body missing %q", want)
		}
	}
	if len(msg.To) != 1 || msg.To[0] != "ops@example.com" {
		t.Errorf("Message.To = %v, want [ops@example.com]", msg.To)
	}
}

func TestFmtTime(t *testing.T) {
	if got := fmtTime(time.Time{}); got != "" {
		t.Errorf("fmtTime(zero) = %q, want empty", got)
	}
	ts := time.Date(2026, 5, 20, 8, 30, 0, 0, time.UTC)
	if got := fmtTime(ts); !strings.Contains(got, "2026-05-20") {
		t.Errorf("fmtTime() = %q, want a 2026-05-20 timestamp", got)
	}
}

func TestNewRendererSelectsTemplate(t *testing.T) {
	for _, name := range TemplateNames() {
		if _, err := NewRenderer("", name); err != nil {
			t.Errorf("NewRenderer(%q) error: %v", name, err)
		}
	}
	// An empty name defaults to the modern template.
	if _, err := NewRenderer("", ""); err != nil {
		t.Errorf("NewRenderer(\"\") error: %v", err)
	}
	// An unknown name is rejected.
	if _, err := NewRenderer("", "bogus"); err == nil {
		t.Error("NewRenderer(\"bogus\") error = nil, want an unknown-template error")
	}
}

func TestRenderClassicTemplate(t *testing.T) {
	r, err := NewRenderer("[Mon]", TemplateClassic)
	if err != nil {
		t.Fatalf("NewRenderer(classic) error: %v", err)
	}
	msg, err := r.Render(Group{
		ExternalURL: "http://alertmanager.example.com",
		GroupLabels: map[string]string{"alertname": "HighCPU"},
		Alerts: []alertmanager.Alert{
			{
				Status:       "firing",
				Labels:       map[string]string{"alertname": "HighCPU", "severity": "warning"},
				Annotations:  map[string]string{"summary": "cpu hot"},
				GeneratorURL: "http://prometheus.example.com/graph",
			},
			{
				Status: "resolved",
				Labels: map[string]string{"alertname": "DiskLow"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Render(classic) error: %v", err)
	}
	// The classic template splits alerts into Firing and Resolved sections.
	for _, want := range []string{"[1] Firing", "[1] Resolved", "HighCPU", "DiskLow"} {
		if !strings.Contains(msg.HTMLBody, want) {
			t.Errorf("classic body missing %q", want)
		}
	}
}

func TestRenderIncludesProductName(t *testing.T) {
	for _, tmpl := range TemplateNames() {
		r, err := NewRenderer("", tmpl)
		if err != nil {
			t.Fatalf("NewRenderer(%q) error: %v", tmpl, err)
		}
		msg, err := r.Render(Group{Alerts: []alertmanager.Alert{{
			Status: "firing",
			Labels: map[string]string{"alertname": "X"},
		}}})
		if err != nil {
			t.Fatalf("Render(%q) error: %v", tmpl, err)
		}
		if !strings.Contains(msg.HTMLBody, branding.ProductName) {
			t.Errorf("%s body missing product name %q", tmpl, branding.ProductName)
		}
	}
}

func TestRenderOmitsEmptyLinks(t *testing.T) {
	// A webhook may arrive without an externalURL, and an alert without a
	// generatorURL. Neither template may then emit a dead link with an empty
	// href; only links actually delivered by the webhook are rendered.
	for _, tmpl := range TemplateNames() {
		r, err := NewRenderer("", tmpl)
		if err != nil {
			t.Fatalf("NewRenderer(%q) error: %v", tmpl, err)
		}
		msg, err := r.Render(Group{
			// ExternalURL is deliberately empty.
			Alerts: []alertmanager.Alert{{
				Status: "firing",
				Labels: map[string]string{"alertname": "X"},
				// GeneratorURL is deliberately empty.
			}},
		})
		if err != nil {
			t.Fatalf("Render(%q) error: %v", tmpl, err)
		}
		if strings.Contains(msg.HTMLBody, `href=""`) {
			t.Errorf("%s body contains a dead link with an empty href", tmpl)
		}
	}
}
