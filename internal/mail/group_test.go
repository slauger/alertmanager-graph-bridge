package mail

import (
	"sort"
	"strings"
	"testing"

	"github.com/slauger/alertmanager-graph-bridge/internal/alertmanager"
)

func alert(name, emailTo string) alertmanager.Alert {
	labels := map[string]string{"alertname": name}
	if emailTo != "" {
		labels["email_to"] = emailTo
	}
	return alertmanager.Alert{Status: "firing", Labels: labels}
}

func TestGroupAlertsRecipientSplitting(t *testing.T) {
	payload := &alertmanager.Payload{
		GroupKey: "group-key",
		Alerts: []alertmanager.Alert{
			alert("A", "team-a@example.com"),
			alert("B", "team-a@example.com"),
			alert("C", "team-a@example.com,team-b@example.com"),
			alert("D", ""),
		},
	}

	groups := GroupAlerts(payload, []string{"default@example.com"})
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}

	// Keys sort to: default@..., team-a@..., team-a@...,team-b@...
	if got := groups[0].Recipients; len(got) != 1 || got[0] != "default@example.com" {
		t.Errorf("groups[0].Recipients = %v, want [default@example.com]", got)
	}
	if len(groups[0].Alerts) != 1 {
		t.Errorf("default group has %d alerts, want 1", len(groups[0].Alerts))
	}
	if len(groups[1].Alerts) != 2 {
		t.Errorf("team-a group has %d alerts, want 2", len(groups[1].Alerts))
	}
	if len(groups[2].Recipients) != 2 {
		t.Errorf("groups[2].Recipients = %v, want 2 entries", groups[2].Recipients)
	}
	for _, g := range groups {
		if g.GroupKey != "group-key" {
			t.Errorf("GroupKey = %q, want group-key", g.GroupKey)
		}
	}
}

func TestGroupAlertsMergesLabelEqualToDefault(t *testing.T) {
	payload := &alertmanager.Payload{
		Alerts: []alertmanager.Alert{
			alert("A", "shared@example.com"),
			alert("B", ""),
		},
	}
	groups := GroupAlerts(payload, []string{"shared@example.com"})
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1 (explicit label equals default)", len(groups))
	}
	if len(groups[0].Alerts) != 2 {
		t.Errorf("merged group has %d alerts, want 2", len(groups[0].Alerts))
	}
}

func TestGroupAlertsDedupAndLowercase(t *testing.T) {
	payload := &alertmanager.Payload{
		Alerts: []alertmanager.Alert{alert("A", "Ops@Example.com, ops@example.com")},
	}
	groups := GroupAlerts(payload, nil)
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(groups))
	}
	if got := groups[0].Recipients; len(got) != 1 || got[0] != "ops@example.com" {
		t.Errorf("Recipients = %v, want [ops@example.com]", got)
	}
}

func TestGroupAlertsDropsInvalidEmailToAddresses(t *testing.T) {
	payload := &alertmanager.Payload{
		Alerts: []alertmanager.Alert{
			// One valid plus one invalid address in the same label.
			alert("A", "valid@example.com, not-an-email"),
			// All addresses invalid: the alert falls back to the default set.
			alert("B", "garbage, also-garbage"),
		},
	}
	groups := GroupAlerts(payload, []string{"default@example.com"})
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}

	keys := map[string]bool{}
	for _, g := range groups {
		keys[strings.Join(g.Recipients, ",")] = true
	}
	if !keys["valid@example.com"] {
		t.Errorf("expected a group routed only to valid@example.com, got %v", keys)
	}
	if !keys["default@example.com"] {
		t.Errorf("expected the all-invalid alert to fall back to default, got %v", keys)
	}
}

func TestGroupAlertsEmpty(t *testing.T) {
	if groups := GroupAlerts(&alertmanager.Payload{}, []string{"x@example.com"}); groups != nil {
		t.Errorf("GroupAlerts(empty payload) = %v, want nil", groups)
	}
	if groups := GroupAlerts(nil, nil); groups != nil {
		t.Errorf("GroupAlerts(nil) = %v, want nil", groups)
	}
}

func TestGroupAlertsDeterministicOrder(t *testing.T) {
	payload := &alertmanager.Payload{
		Alerts: []alertmanager.Alert{
			alert("A", "z@example.com"),
			alert("B", "a@example.com"),
			alert("C", "m@example.com"),
		},
	}
	groups := GroupAlerts(payload, nil)
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}

	keys := make([]string, len(groups))
	for i, g := range groups {
		keys[i] = strings.Join(g.Recipients, ",")
	}
	if !sort.StringsAreSorted(keys) {
		t.Errorf("groups not in deterministic sorted order: %v", keys)
	}
}
