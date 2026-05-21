//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/slauger/alertmanager-graph-bridge/internal/config"
	"github.com/slauger/alertmanager-graph-bridge/internal/graph"
	"github.com/slauger/alertmanager-graph-bridge/internal/mail"
	"github.com/slauger/alertmanager-graph-bridge/internal/server"
)

// liveConfig is the configuration for the live Microsoft Graph tests.
type liveConfig struct {
	tenantID     string
	clientID     string
	clientSecret string
	mailFrom     string
	mailTo       string
}

// loadConfig reads the live test configuration from the environment. Any
// missing variable skips the test, so the suite is a safe no-op without
// credentials.
func loadConfig(t *testing.T) liveConfig {
	t.Helper()
	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			t.Skipf("e2e: %s is not set; skipping live Microsoft Graph test", key)
		}
		return v
	}
	return liveConfig{
		tenantID:     get("AGB_AZURE_TENANTID"),
		clientID:     get("AGB_AZURE_CLIENTID"),
		clientSecret: get("AGB_AZURE_CLIENTSECRET"),
		mailFrom:     get("E2E_MAIL_FROM"),
		mailTo:       get("E2E_MAIL_TO"),
	}
}

// startBridge wires the real Graph client and HTTP server exactly as the
// production binary does, rendering e-mails with the named template, and
// returns a running test server. The template name is woven into the subject
// prefix so the resulting e-mails are easy to tell apart in the mailbox.
func startBridge(t *testing.T, cfg liveConfig, bearerToken, tmpl string) *httptest.Server {
	t.Helper()

	graphClient, err := graph.New(graph.Options{
		TenantID:     cfg.tenantID,
		ClientID:     cfg.clientID,
		ClientSecret: cfg.clientSecret,
		From:         cfg.mailFrom,
		Timeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("graph.New: %v", err)
	}

	renderer, err := mail.NewRenderer("[AGB-E2E "+tmpl+"]", tmpl)
	if err != nil {
		t.Fatalf("mail.NewRenderer: %v", err)
	}

	srv := server.New(server.Options{
		Cfg:         config.ServerConfig{BearerToken: bearerToken},
		Sender:      graphClient,
		Renderer:    renderer,
		DefaultTo:   []string{cfg.mailTo},
		SendTimeout: 60 * time.Second,
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// postWebhook sends an Alertmanager webhook payload to the bridge.
func postWebhook(t *testing.T, ts *httptest.Server, bearer, body string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ts.URL+"/api/v1/alerts", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(respBody)
}

// marker returns a unique, timestamped identifier so the resulting e-mails are
// easy to find in the recipient mailbox.
func marker() string {
	return time.Now().UTC().Format("20060102T150405.000Z")
}

// bogusSender returns a non-existent mailbox address in the same domain as the
// given address, used to provoke a Microsoft Graph error.
func bogusSender(t *testing.T, from string) string {
	t.Helper()
	at := strings.LastIndex(from, "@")
	if at < 0 {
		t.Fatalf("E2E_MAIL_FROM %q is not a valid address", from)
	}
	return fmt.Sprintf("agb-e2e-nonexistent-%s%s", marker(), from[at:])
}

func firingWebhook(id string) string {
	return fmt.Sprintf(`{
  "version": "4",
  "groupKey": "e2e-firing-%[1]s",
  "status": "firing",
  "receiver": "graph-bridge",
  "externalURL": "https://alertmanager.example.com",
  "alerts": [
    {
      "status": "firing",
      "labels": {"alertname": "E2EFiringAlert", "severity": "warning", "run": "%[1]s"},
      "annotations": {"summary": "e2e firing alert %[1]s", "description": "Automated end-to-end test."},
      "startsAt": "%[2]s",
      "endsAt": "0001-01-01T00:00:00Z",
      "generatorURL": "https://prometheus.example.com/graph"
    }
  ]
}`, id, time.Now().UTC().Format(time.RFC3339))
}

func resolvedWebhook(id string) string {
	now := time.Now().UTC()
	return fmt.Sprintf(`{
  "version": "4",
  "groupKey": "e2e-resolved-%[1]s",
  "status": "resolved",
  "receiver": "graph-bridge",
  "externalURL": "https://alertmanager.example.com",
  "alerts": [
    {
      "status": "resolved",
      "labels": {"alertname": "E2EResolvedAlert", "severity": "critical", "run": "%[1]s"},
      "annotations": {"summary": "e2e resolved alert %[1]s"},
      "startsAt": "%[2]s",
      "endsAt": "%[3]s",
      "generatorURL": "https://prometheus.example.com/graph"
    }
  ]
}`, id, now.Add(-time.Hour).Format(time.RFC3339), now.Format(time.RFC3339))
}

func groupedWebhook(id string) string {
	return fmt.Sprintf(`{
  "version": "4",
  "groupKey": "e2e-grouped-%[1]s",
  "status": "firing",
  "receiver": "graph-bridge",
  "externalURL": "https://alertmanager.example.com",
  "alerts": [
    {
      "status": "firing",
      "labels": {"alertname": "E2EGroupedAlert", "severity": "warning", "instance": "node-1", "run": "%[1]s"},
      "annotations": {"summary": "e2e grouped alert one %[1]s"},
      "startsAt": "%[2]s",
      "endsAt": "0001-01-01T00:00:00Z"
    },
    {
      "status": "firing",
      "labels": {"alertname": "E2EGroupedAlert", "severity": "critical", "instance": "node-2", "run": "%[1]s"},
      "annotations": {"summary": "e2e grouped alert two %[1]s"},
      "startsAt": "%[2]s",
      "endsAt": "0001-01-01T00:00:00Z"
    }
  ]
}`, id, time.Now().UTC().Format(time.RFC3339))
}

// mixedRecipientWebhook has one alert that overrides the recipient via the
// email_to label and one that uses the default recipient. When the two
// addresses differ the bridge must fan out into two separate e-mails.
func mixedRecipientWebhook(id, override string) string {
	return fmt.Sprintf(`{
  "version": "4",
  "groupKey": "e2e-override-%[1]s",
  "status": "firing",
  "receiver": "graph-bridge",
  "externalURL": "https://alertmanager.example.com",
  "alerts": [
    {
      "status": "firing",
      "labels": {"alertname": "E2EOverrideAlert", "severity": "warning", "run": "%[1]s", "email_to": "%[3]s"},
      "annotations": {"summary": "e2e email_to override %[1]s"},
      "startsAt": "%[2]s",
      "endsAt": "0001-01-01T00:00:00Z"
    },
    {
      "status": "firing",
      "labels": {"alertname": "E2EDefaultAlert", "severity": "warning", "run": "%[1]s"},
      "annotations": {"summary": "e2e default recipient %[1]s"},
      "startsAt": "%[2]s",
      "endsAt": "0001-01-01T00:00:00Z"
    }
  ]
}`, id, time.Now().UTC().Format(time.RFC3339), override)
}

// assertAccepted fails the test unless the bridge accepted and forwarded the
// webhook. HTTP 200 means every Microsoft Graph sendMail call returned 202
// Accepted; it does not guarantee asynchronous delivery by Exchange Online.
func assertAccepted(t *testing.T, status int, body, scenario string) {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("%s: bridge returned %d (%s); Microsoft Graph did not accept the send",
			scenario, status, strings.TrimSpace(body))
	}
}

// assertRejected fails the test unless the bridge reported a downstream Graph
// failure as HTTP 502.
func assertRejected(t *testing.T, status int, scenario string) {
	t.Helper()
	if status != http.StatusBadGateway {
		t.Fatalf("%s: bridge returned %d, want 502", scenario, status)
	}
}

// The five accepted-delivery scenarios are split across the two e-mail
// templates, so a full run sends five e-mails that together exercise both the
// modern and the classic layout.

func TestE2EFiringAlertAccepted(t *testing.T) {
	cfg := loadConfig(t)
	const tmpl = mail.TemplateModern
	id := marker()
	status, body := postWebhook(t, startBridge(t, cfg, "", tmpl), "", firingWebhook(id))
	assertAccepted(t, status, body, "firing alert")
	t.Logf("sent firing-alert e-mail (%s template) to %s (run %s)", tmpl, cfg.mailTo, id)
}

func TestE2EResolvedAlertAccepted(t *testing.T) {
	cfg := loadConfig(t)
	const tmpl = mail.TemplateClassic
	id := marker()
	status, body := postWebhook(t, startBridge(t, cfg, "", tmpl), "", resolvedWebhook(id))
	assertAccepted(t, status, body, "resolved alert")
	t.Logf("sent resolved-alert e-mail (%s template) to %s (run %s)", tmpl, cfg.mailTo, id)
}

func TestE2EGroupedAlertsAccepted(t *testing.T) {
	cfg := loadConfig(t)
	const tmpl = mail.TemplateModern
	id := marker()
	status, body := postWebhook(t, startBridge(t, cfg, "", tmpl), "", groupedWebhook(id))
	assertAccepted(t, status, body, "grouped alerts")
	t.Logf("sent grouped-alerts e-mail (%s template) to %s (run %s)", tmpl, cfg.mailTo, id)
}

func TestE2ERecipientOverrideFanOut(t *testing.T) {
	cfg := loadConfig(t)
	const tmpl = mail.TemplateClassic
	id := marker()
	// The override alert is routed to the sender mailbox, which is a known,
	// valid address. When it differs from the default recipient this exercises
	// live fan-out: one webhook produces two separate e-mails.
	status, body := postWebhook(t, startBridge(t, cfg, "", tmpl), "", mixedRecipientWebhook(id, cfg.mailFrom))
	assertAccepted(t, status, body, "recipient override fan-out")
	if strings.EqualFold(cfg.mailFrom, cfg.mailTo) {
		t.Logf("sent override e-mail (%s template, run %s); set distinct E2E_MAIL_FROM/E2E_MAIL_TO to exercise fan-out", tmpl, id)
	} else {
		t.Logf("sent two e-mails (%s template): override -> %s, default -> %s (run %s)", tmpl, cfg.mailFrom, cfg.mailTo, id)
	}
}

func TestE2EBearerAuthentication(t *testing.T) {
	cfg := loadConfig(t)
	const token = "e2e-bearer-token"
	const tmpl = mail.TemplateModern
	ts := startBridge(t, cfg, token, tmpl)

	// A request without the token must be rejected before any Graph call.
	if status, _ := postWebhook(t, ts, "", firingWebhook(marker())); status != http.StatusUnauthorized {
		t.Fatalf("missing token: status = %d, want 401", status)
	}

	// With the correct token the alert is forwarded.
	id := marker()
	status, body := postWebhook(t, ts, token, firingWebhook(id))
	assertAccepted(t, status, body, "authenticated request")
	t.Logf("sent authenticated e-mail (%s template) to %s (run %s)", tmpl, cfg.mailTo, id)
}

func TestE2EInvalidSenderReturns502(t *testing.T) {
	cfg := loadConfig(t)
	// A non-existent sender mailbox makes Microsoft Graph reject the request;
	// the bridge must surface that as HTTP 502. This exercises the live
	// error-handling path (parseAPIError, classifyError, the 4xx -> 502 map).
	// The error path is independent of the e-mail template.
	bad := cfg
	bad.mailFrom = bogusSender(t, cfg.mailFrom)
	status, _ := postWebhook(t, startBridge(t, bad, "", mail.TemplateModern), "", firingWebhook(marker()))
	assertRejected(t, status, "invalid sender mailbox")
}

func TestE2EInvalidCredentialsReturn502(t *testing.T) {
	cfg := loadConfig(t)
	// A wrong client secret makes the OAuth2 token request fail; the bridge
	// must surface that as HTTP 502 rather than panicking. The error path is
	// independent of the e-mail template.
	bad := cfg
	bad.clientSecret = "deliberately-invalid-client-secret"
	status, _ := postWebhook(t, startBridge(t, bad, "", mail.TemplateModern), "", firingWebhook(marker()))
	assertRejected(t, status, "invalid client secret")
}
