package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/alertmanager"
	"github.com/slauger/alertmanager-graph-bridge/internal/branding"
)

// Renderer renders alert groups into HTML e-mail messages.
type Renderer struct {
	tmpl          *template.Template
	subjectPrefix string
}

// NewRenderer parses the named HTML template and returns a Renderer. The
// subjectPrefix is prepended to every generated subject line. templateName
// selects the e-mail layout; an empty name selects the modern default.
func NewRenderer(subjectPrefix, templateName string) (*Renderer, error) {
	if templateName == "" {
		templateName = TemplateModern
	}
	src, ok := templateSources[templateName]
	if !ok {
		return nil, fmt.Errorf("mail: unknown template %q (want one of %s)",
			templateName, strings.Join(TemplateNames(), ", "))
	}
	tmpl, err := template.New("email").Funcs(template.FuncMap{
		"fmtTime":     fmtTime,
		"sortedPairs": sortedPairs,
	}).Parse(src)
	if err != nil {
		return nil, fmt.Errorf("mail: parsing %s template: %w", templateName, err)
	}
	return &Renderer{tmpl: tmpl, subjectPrefix: strings.TrimSpace(subjectPrefix)}, nil
}

// renderData is the view model handed to the HTML template.
type renderData struct {
	// ProductName is the user-visible brand, shown in the title and footer.
	ProductName string
	Status      string
	Count       int
	ExternalURL string
	// GroupLabels are the Alertmanager group labels shared by the alerts.
	GroupLabels map[string]string
	// Alerts holds every alert in the group; Firing and Resolved are the
	// same alerts partitioned by status. The modern template uses Alerts,
	// the classic template uses Firing and Resolved.
	Alerts   []alertmanager.Alert
	Firing   []alertmanager.Alert
	Resolved []alertmanager.Alert
}

// pair is a sorted key/value entry used for labels and annotations.
type pair struct {
	Key   string
	Value string
}

// Render produces an e-mail Message for a single recipient group.
func (r *Renderer) Render(g Group) (Message, error) {
	firing, resolved := partitionByStatus(g.Alerts)
	data := renderData{
		ProductName: branding.ProductName,
		Status:      groupStatus(g.Alerts),
		Count:       len(g.Alerts),
		ExternalURL: g.ExternalURL,
		GroupLabels: g.GroupLabels,
		Alerts:      g.Alerts,
		Firing:      firing,
		Resolved:    resolved,
	}
	var buf bytes.Buffer
	if err := r.tmpl.Execute(&buf, data); err != nil {
		return Message{}, fmt.Errorf("mail: rendering template: %w", err)
	}
	return Message{
		Subject:  r.subject(g),
		HTMLBody: buf.String(),
		To:       g.Recipients,
	}, nil
}

// partitionByStatus splits alerts into firing and resolved, preserving order.
func partitionByStatus(alerts []alertmanager.Alert) (firing, resolved []alertmanager.Alert) {
	for _, a := range alerts {
		if a.IsFiring() {
			firing = append(firing, a)
		} else {
			resolved = append(resolved, a)
		}
	}
	return firing, resolved
}

// subject builds the subject line: "{prefix} [{STATUS}] {alertname} - {summary}".
func (r *Renderer) subject(g Group) string {
	prefix := r.subjectPrefix
	if prefix != "" {
		prefix += " "
	}
	status := groupStatus(g.Alerts)

	if len(g.Alerts) == 1 {
		a := g.Alerts[0]
		s := fmt.Sprintf("%s[%s] %s", prefix, status, a.Name())
		if sum := a.Summary(); sum != "" {
			s += " - " + sum
		}
		return s
	}

	if name := commonName(g.Alerts); name != "" {
		return fmt.Sprintf("%s[%s] %s (%d alerts)", prefix, status, name, len(g.Alerts))
	}
	return fmt.Sprintf("%s[%s] %d alerts", prefix, status, len(g.Alerts))
}

// groupStatus reports the aggregate status of a set of alerts.
func groupStatus(alerts []alertmanager.Alert) string {
	var firing, resolved bool
	for _, a := range alerts {
		if a.IsFiring() {
			firing = true
		} else {
			resolved = true
		}
	}
	switch {
	case firing && resolved:
		return "FIRING/RESOLVED"
	case resolved:
		return "RESOLVED"
	default:
		return "FIRING"
	}
}

// commonName returns the shared alertname when all alerts have one, else "".
func commonName(alerts []alertmanager.Alert) string {
	if len(alerts) == 0 {
		return ""
	}
	name := alerts[0].Name()
	for _, a := range alerts[1:] {
		if a.Name() != name {
			return ""
		}
	}
	return name
}

// fmtTime formats a timestamp for display, returning "" for the zero value.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05 MST")
}

// sortedPairs converts a label/annotation map into a key-sorted slice.
func sortedPairs(m map[string]string) []pair {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]pair, 0, len(keys))
	for _, k := range keys {
		out = append(out, pair{Key: k, Value: m[k]})
	}
	return out
}
