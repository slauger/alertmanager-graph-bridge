// Package mail buckets Alertmanager alerts by recipient set and renders them
// into HTML e-mail messages.
package mail

import (
	netmail "net/mail"
	"sort"
	"strings"

	"github.com/slauger/alertmanager-graph-bridge/internal/alertmanager"
)

// Message is a rendered e-mail ready to be handed to the Graph client.
type Message struct {
	Subject  string
	HTMLBody string
	To       []string
}

// Group is a set of alerts that share the same recipient set.
type Group struct {
	// GroupKey is the Alertmanager groupKey the alerts originated from.
	GroupKey string
	// ExternalURL links back to the Alertmanager UI.
	ExternalURL string
	// GroupLabels are the Alertmanager group labels for the webhook.
	GroupLabels map[string]string
	// Recipients is the sorted, de-duplicated recipient list.
	Recipients []string
	// Alerts are the alerts destined for Recipients.
	Alerts []alertmanager.Alert
}

// GroupAlerts buckets the payload's alerts by their resolved recipient set. An
// alert's recipients are taken from its email_to label when present, otherwise
// from defaultTo. Groups are returned in a deterministic order.
func GroupAlerts(p *alertmanager.Payload, defaultTo []string) []Group {
	if p == nil || len(p.Alerts) == 0 {
		return nil
	}

	defaultSet := normalizeRecipients(defaultTo)
	buckets := make(map[string][]alertmanager.Alert)

	for _, a := range p.Alerts {
		recipients := normalizeRecipients(a.EmailTo())
		if len(recipients) == 0 {
			recipients = defaultSet
		}
		key := strings.Join(recipients, ",")
		buckets[key] = append(buckets[key], a)
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	groups := make([]Group, 0, len(keys))
	for _, k := range keys {
		var recipients []string
		if k != "" {
			recipients = strings.Split(k, ",")
		}
		groups = append(groups, Group{
			GroupKey:    p.GroupKey,
			ExternalURL: p.ExternalURL,
			GroupLabels: p.GroupLabels,
			Recipients:  recipients,
			Alerts:      buckets[k],
		})
	}
	return groups
}

// normalizeRecipients trims, validates, lowercases, de-duplicates and sorts
// addresses. Entries that are not valid e-mail addresses are dropped, so a
// malformed email_to label cannot break delivery for a whole group.
func normalizeRecipients(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := netmail.ParseAddress(raw)
		if err != nil {
			continue
		}
		addr := strings.ToLower(parsed.Address)
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}
