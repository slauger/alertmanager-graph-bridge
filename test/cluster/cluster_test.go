//go:build cluster

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Alert names defined in test/cluster/manifests/10-prometheus.yaml.
const (
	firingAlert   = "E2EFiringAlert"
	overrideAlert = "E2EOverrideAlert"
	groupedAlert  = "E2EGroupedAlert"
)

// firingGroups is the number of e-mails one firing run produces: one per
// Alertmanager group -- E2EFiringAlert, E2EOverrideAlert and E2EGroupedAlert
// (the last bundles two alerts into a single e-mail).
const firingGroups = 3

// minimalWebhook is a syntactically valid Alertmanager webhook used by the
// authentication test; its content is irrelevant because the auth middleware
// runs before the payload is parsed.
const minimalWebhook = `{"version":"4","groupKey":"auth-probe","status":"firing","alerts":[]}`

// clusterConfig holds the endpoints of the deployed stack. hack/cluster-e2e.sh
// port-forwards each service to localhost and exports these variables.
type clusterConfig struct {
	prometheusURL   string
	alertmanagerURL string
	bridgeURL       string
	subjectMarker   string
	namespace       string
	bridgeDeploy    string
	overrideMail    string
}

// loadClusterConfig reads the cluster endpoints from the environment. A missing
// variable skips the test, so the suite is a safe no-op without a cluster.
func loadClusterConfig(t *testing.T) clusterConfig {
	t.Helper()
	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			t.Skipf("cluster: %s is not set; skipping full-chain cluster test", key)
		}
		return v
	}
	return clusterConfig{
		prometheusURL:   get("CLUSTER_PROMETHEUS_URL"),
		alertmanagerURL: get("CLUSTER_ALERTMANAGER_URL"),
		bridgeURL:       get("CLUSTER_BRIDGE_URL"),
		namespace:       get("CLUSTER_NAMESPACE"),
		bridgeDeploy:    get("CLUSTER_BRIDGE_DEPLOY"),
		overrideMail:    strings.ToLower(get("CLUSTER_OVERRIDE_MAIL")),
		subjectMarker:   os.Getenv("CLUSTER_SUBJECT_MARKER"),
	}
}

// --- HTTP / polling helpers -------------------------------------------------

// tryGet performs a short-lived GET and returns the body only on HTTP 200.
// Transient errors are returned to the caller, which retries via poll.
func tryGet(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return body, nil
}

// poll calls check every two seconds until it succeeds or timeout elapses,
// failing the test with the last observed status on timeout.
func poll(t *testing.T, what string, timeout time.Duration, check func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := "no attempt completed"
	for time.Now().Before(deadline) {
		ok, status := check()
		last = status
		if ok {
			t.Logf("%s: %s", what, status)
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("%s: timed out after %s (last: %s)", what, timeout, last)
}

// --- bridge metrics ---------------------------------------------------------

// sumMetric sums every Prometheus text-format series of metric name whose line
// contains labelFilter (pass "" to match every series of that metric).
func sumMetric(text, name, labelFilter string) float64 {
	var total float64
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		series, valStr := line[:sp], line[sp+1:]
		if series != name && !strings.HasPrefix(series, name+"{") {
			continue
		}
		if labelFilter != "" && !strings.Contains(series, labelFilter) {
			continue
		}
		if v, err := strconv.ParseFloat(valStr, 64); err == nil {
			total += v
		}
	}
	return total
}

// mailsSent reads the bridge /metrics endpoint and returns agb_mails_sent_total,
// failing the test if the bridge reported any mail send error.
func mailsSent(t *testing.T, cfg clusterConfig) float64 {
	t.Helper()
	body, err := tryGet(cfg.bridgeURL + "/metrics")
	if err != nil {
		t.Fatalf("reading bridge metrics: %v", err)
	}
	metrics := string(body)
	if errs := sumMetric(metrics, "agb_mail_send_errors_total", ""); errs > 0 {
		t.Fatalf("bridge reported %.0f mail send error(s); inspect `kubectl logs` for the bridge pod", errs)
	}
	return sumMetric(metrics, "agb_mails_sent_total", "")
}

// --- bridge logs ------------------------------------------------------------

// kubectl runs a kubectl command and returns its combined output.
func kubectl(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "kubectl", args...).CombinedOutput()
	return string(out), err
}

// deliveredGroup mirrors a bridge "delivered alert group" structured log line.
type deliveredGroup struct {
	Msg        string   `json:"msg"`
	GroupKey   string   `json:"groupKey"`
	Recipients []string `json:"recipients"`
	Alerts     int      `json:"alerts"`
}

// bridgeDeliveries parses the bridge pod logs and returns every successfully
// delivered alert group.
func bridgeDeliveries(t *testing.T, cfg clusterConfig) []deliveredGroup {
	t.Helper()
	out, err := kubectl(t, "logs", "deployment/"+cfg.bridgeDeploy,
		"-n", cfg.namespace, "--tail=-1")
	if err != nil {
		t.Logf("kubectl logs failed (will retry): %v", err)
		return nil
	}
	var groups []deliveredGroup
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var g deliveredGroup
		if json.Unmarshal([]byte(line), &g) != nil {
			continue
		}
		if g.Msg == "delivered alert group" {
			groups = append(groups, g)
		}
	}
	return groups
}

// --- tests (run in source order; the suite is designed as one full run) -----

// TestClusterAlertFiresInPrometheus verifies the first hop: Prometheus
// evaluates the rules and the synthetic alert becomes firing.
func TestClusterAlertFiresInPrometheus(t *testing.T) {
	cfg := loadClusterConfig(t)
	poll(t, "alert firing in Prometheus", 90*time.Second, func() (bool, string) {
		body, err := tryGet(cfg.prometheusURL + "/api/v1/alerts")
		if err != nil {
			return false, err.Error()
		}
		var payload struct {
			Data struct {
				Alerts []struct {
					Labels map[string]string `json:"labels"`
					State  string            `json:"state"`
				} `json:"alerts"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return false, "decode: " + err.Error()
		}
		for _, a := range payload.Data.Alerts {
			if a.Labels["alertname"] == firingAlert && a.State == "firing" {
				return true, firingAlert + " is firing"
			}
		}
		return false, fmt.Sprintf("%d alert(s) seen, %s not firing yet", len(payload.Data.Alerts), firingAlert)
	})
}

// TestClusterAlertReachesAlertmanager verifies the second hop: Prometheus has
// pushed the alert into Alertmanager.
func TestClusterAlertReachesAlertmanager(t *testing.T) {
	cfg := loadClusterConfig(t)
	poll(t, "alert visible in Alertmanager", 90*time.Second, func() (bool, string) {
		body, err := tryGet(cfg.alertmanagerURL + "/api/v2/alerts")
		if err != nil {
			return false, err.Error()
		}
		var alerts []struct {
			Labels map[string]string `json:"labels"`
		}
		if err := json.Unmarshal(body, &alerts); err != nil {
			return false, "decode: " + err.Error()
		}
		for _, a := range alerts {
			if a.Labels["alertname"] == firingAlert {
				return true, firingAlert + " received by Alertmanager"
			}
		}
		return false, fmt.Sprintf("%d alert(s) seen, %s not received yet", len(alerts), firingAlert)
	})
}

// TestClusterWebhookAuthIsEnforced verifies the deployed bridge rejects a
// webhook without a valid bearer token. The positive auth case is proven by
// the rest of the suite: Alertmanager delivers with the token and the mails
// go through.
func TestClusterWebhookAuthIsEnforced(t *testing.T) {
	cfg := loadClusterConfig(t)
	post := func(bearer string) int {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			cfg.bridgeURL+"/api/v1/alerts", strings.NewReader(minimalWebhook))
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("posting webhook: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	if code := post(""); code != http.StatusUnauthorized {
		t.Errorf("webhook without a token: status = %d, want 401", code)
	}
	if code := post("wrong-token"); code != http.StatusUnauthorized {
		t.Errorf("webhook with a wrong token: status = %d, want 401", code)
	}
}

// TestClusterFiringMailsDelivered verifies the final hops for the firing run:
// Alertmanager dispatched the authenticated webhooks, the bridge accepted them
// and Microsoft Graph accepted every sendMail call.
func TestClusterFiringMailsDelivered(t *testing.T) {
	cfg := loadClusterConfig(t)
	if cfg.subjectMarker != "" {
		t.Logf("watch the test mailbox for e-mails whose subject starts with %q", cfg.subjectMarker)
	}
	poll(t, "firing alerts delivered to Microsoft Graph", 240*time.Second, func() (bool, string) {
		body, err := tryGet(cfg.bridgeURL + "/metrics")
		if err != nil {
			return false, err.Error()
		}
		metrics := string(body)
		if errs := sumMetric(metrics, "agb_mail_send_errors_total", ""); errs > 0 {
			return true, fmt.Sprintf("FAIL: bridge reported %.0f mail send error(s)", errs)
		}
		sent := sumMetric(metrics, "agb_mails_sent_total", "")
		if sent >= firingGroups {
			return true, fmt.Sprintf("agb_mails_sent_total = %.0f", sent)
		}
		return false, fmt.Sprintf("agb_mails_sent_total = %.0f, want %d", sent, firingGroups)
	})

	body, err := tryGet(cfg.bridgeURL + "/metrics")
	if err != nil {
		t.Fatalf("reading bridge metrics: %v", err)
	}
	metrics := string(body)
	if errs := sumMetric(metrics, "agb_mail_send_errors_total", ""); errs > 0 {
		t.Fatalf("bridge reported %.0f mail send error(s); inspect `kubectl logs` for the bridge pod", errs)
	}
	if sent := sumMetric(metrics, "agb_mails_sent_total", ""); sent < firingGroups {
		t.Fatalf("agb_mails_sent_total = %.0f, want >= %d", sent, firingGroups)
	}
	if ok := sumMetric(metrics, "agb_webhook_requests_total", `outcome="ok"`); ok < firingGroups {
		t.Errorf(`agb_webhook_requests_total{outcome="ok"} = %.0f, want >= %d`, ok, firingGroups)
	}
}

// TestClusterEmailToOverrideRouted verifies that the email_to label survived
// the Prometheus -> Alertmanager -> webhook path and the bridge routed the
// e-mail to the overridden mailbox. It reads the bridge's structured logs.
func TestClusterEmailToOverrideRouted(t *testing.T) {
	cfg := loadClusterConfig(t)
	poll(t, "email_to override routed by the bridge", 180*time.Second, func() (bool, string) {
		for _, g := range bridgeDeliveries(t, cfg) {
			if !strings.Contains(g.GroupKey, overrideAlert) {
				continue
			}
			if len(g.Recipients) == 1 && strings.EqualFold(g.Recipients[0], cfg.overrideMail) {
				return true, fmt.Sprintf("%s delivered to overridden recipient %s", overrideAlert, cfg.overrideMail)
			}
			return true, fmt.Sprintf("FAIL: %s delivered to %v, want [%s]", overrideAlert, g.Recipients, cfg.overrideMail)
		}
		return false, "no delivered group for " + overrideAlert + " yet"
	})

	for _, g := range bridgeDeliveries(t, cfg) {
		if strings.Contains(g.GroupKey, overrideAlert) {
			if len(g.Recipients) != 1 || !strings.EqualFold(g.Recipients[0], cfg.overrideMail) {
				t.Fatalf("%s recipients = %v, want [%s]", overrideAlert, g.Recipients, cfg.overrideMail)
			}
			return
		}
	}
	t.Fatalf("no delivered alert group for %s found in the bridge logs", overrideAlert)
}

// TestClusterGroupedAlertsCollapsedIntoOneMail verifies that two alerts sharing
// an alertname were grouped by Alertmanager into one webhook and delivered by
// the bridge as a single e-mail covering both alerts.
func TestClusterGroupedAlertsCollapsedIntoOneMail(t *testing.T) {
	cfg := loadClusterConfig(t)
	poll(t, "grouped alerts collapsed into one e-mail", 180*time.Second, func() (bool, string) {
		matches := 0
		var found deliveredGroup
		for _, g := range bridgeDeliveries(t, cfg) {
			if strings.Contains(g.GroupKey, groupedAlert) {
				matches++
				found = g
			}
		}
		if matches == 0 {
			return false, "no delivered group for " + groupedAlert + " yet"
		}
		if matches == 1 && found.Alerts == 2 {
			return true, fmt.Sprintf("%s delivered as one e-mail covering %d alerts", groupedAlert, found.Alerts)
		}
		return true, fmt.Sprintf("FAIL: %s produced %d delivered group(s), last with %d alert(s); want 1 group of 2",
			groupedAlert, matches, found.Alerts)
	})

	matches, alerts := 0, 0
	for _, g := range bridgeDeliveries(t, cfg) {
		if strings.Contains(g.GroupKey, groupedAlert) {
			matches++
			alerts = g.Alerts
		}
	}
	if matches != 1 || alerts != 2 {
		t.Fatalf("%s: %d delivered group(s) with %d alert(s); want exactly 1 group of 2",
			groupedAlert, matches, alerts)
	}
}

// TestClusterResolvedNotificationsDelivered verifies the send_resolved path.
// It waits for the firing run to finish, removes the Prometheus alert rules so
// the alerts stop firing, and confirms Alertmanager dispatches resolved
// webhooks that the bridge delivers. After the rules are gone, the only
// webhooks Alertmanager can send are resolved ones, so a rise in
// agb_mails_sent_total is an unambiguous resolved-delivery signal.
func TestClusterResolvedNotificationsDelivered(t *testing.T) {
	cfg := loadClusterConfig(t)

	// Make sure the firing run has completed before tearing the rules down.
	poll(t, "firing run complete before resolving", 240*time.Second, func() (bool, string) {
		sent := mailsSent(t, cfg)
		if sent >= firingGroups {
			return true, fmt.Sprintf("agb_mails_sent_total = %.0f", sent)
		}
		return false, fmt.Sprintf("agb_mails_sent_total = %.0f, want %d", sent, firingGroups)
	})
	base := mailsSent(t, cfg)

	// Remove every alert rule, then restart Prometheus so it reloads the now
	// empty rule set. The active alerts then age out and Alertmanager resolves
	// them.
	t.Log("removing the Prometheus alert rules to trigger resolved notifications")
	if out, err := kubectl(t, "-n", cfg.namespace, "patch", "configmap", "prometheus",
		"--type", "merge", "-p", `{"data":{"rules.yml":"groups: []\n"}}`); err != nil {
		t.Fatalf("patching the prometheus ConfigMap: %v\n%s", err, out)
	}
	if out, err := kubectl(t, "-n", cfg.namespace, "rollout", "restart", "deployment/prometheus"); err != nil {
		t.Fatalf("restarting Prometheus: %v\n%s", err, out)
	}
	if out, err := kubectl(t, "-n", cfg.namespace, "rollout", "status",
		"deployment/prometheus", "--timeout=120s"); err != nil {
		t.Fatalf("waiting for the Prometheus restart: %v\n%s", err, out)
	}

	want := base + firingGroups
	poll(t, "resolved notifications delivered to Microsoft Graph", 300*time.Second, func() (bool, string) {
		body, err := tryGet(cfg.bridgeURL + "/metrics")
		if err != nil {
			return false, err.Error()
		}
		metrics := string(body)
		if errs := sumMetric(metrics, "agb_mail_send_errors_total", ""); errs > 0 {
			return true, fmt.Sprintf("FAIL: bridge reported %.0f mail send error(s)", errs)
		}
		sent := sumMetric(metrics, "agb_mails_sent_total", "")
		if sent >= want {
			return true, fmt.Sprintf("agb_mails_sent_total = %.0f (resolved run added %.0f)", sent, sent-base)
		}
		return false, fmt.Sprintf("agb_mails_sent_total = %.0f, want %.0f", sent, want)
	})

	if sent := mailsSent(t, cfg); sent < want {
		t.Fatalf("agb_mails_sent_total = %.0f after resolve, want >= %.0f", sent, want)
	}
}
