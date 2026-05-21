# Cluster End-to-End Testing

The cluster end-to-end test exercises the **complete delivery chain** inside a
real, ephemeral Kubernetes cluster. Where the [e2e suite](e2e-testing.md) posts
a hand-crafted webhook straight at the bridge, this test makes a real
Prometheus generate an alert and a real Alertmanager dispatch it:

```text
Prometheus  --fires rule-->  Alertmanager  --webhook (Bearer auth)-->  bridge
                                                                         |
                                              live Microsoft Graph <-----+
                                                       |
                                                       v
                                                  test mailbox
```

Everything is fully automated: a single command provisions a
[kind](https://kind.sigs.k8s.io/) cluster, builds and loads the bridge image,
deploys Prometheus + Alertmanager + the bridge, runs the assertions, and tears
the cluster down again.

## TL;DR

```bash
cp e2e.env.example e2e.env   # fill in the Terraform outputs + mailboxes
make cluster-e2e             # provision, deploy, test, tear down
```

## What you need

The Microsoft 365 / Entra prerequisites are identical to the
[e2e suite](e2e-testing.md#what-you-need) - the same Entra app registration and
the same five variables in `e2e.env`. Provision them once with
`make e2e-infra` (Terraform).

Locally you need **only Docker**. The `cluster-e2e` target builds a small
*orchestrator image* (`images/cluster-e2e/Containerfile`) that bundles `kind`,
`kubectl`, `helm` and the Go toolchain, and runs it with the host Docker socket
mounted - so kind can create the cluster as sibling containers. Nothing else
has to be installed on the host.

### Cost

The same as the e2e suite: effectively zero. The kind cluster is local and
ephemeral. Each run sends **six** real e-mails to the test mailboxes - three
firing notifications and three resolved notifications.

## What the test covers

The `cluster`-tagged Go tests in [`test/cluster/`](https://github.com/slauger/alertmanager-graph-bridge/tree/main/test/cluster)
walk the whole chain and assert each relevant behaviour:

- `TestClusterAlertFiresInPrometheus` - Prometheus evaluates the rules and the
  synthetic alert becomes `firing`.
- `TestClusterAlertReachesAlertmanager` - Alertmanager has received the alert.
- `TestClusterWebhookAuthIsEnforced` - the deployed bridge rejects a webhook
  sent without a valid bearer token (HTTP 401).
- `TestClusterFiringMailsDelivered` - the bridge metrics confirm every firing
  webhook was accepted (`agb_webhook_requests_total{outcome="ok"}`), Microsoft
  Graph accepted each send (`agb_mails_sent_total`), and there were no errors.
- `TestClusterEmailToOverrideRouted` - the `email_to` label survived the
  Prometheus → Alertmanager → webhook path and the bridge routed that e-mail to
  the overridden mailbox (asserted from the bridge's structured logs).
- `TestClusterGroupedAlertsCollapsedIntoOneMail` - two alerts sharing an
  alertname were grouped by Alertmanager into one webhook and delivered as a
  single e-mail covering both.
- `TestClusterResolvedNotificationsDelivered` - removing the alert rules makes
  Alertmanager dispatch resolved webhooks (`send_resolved`), which the bridge
  delivers too.

Verification combines the bridge's Prometheus metrics with its structured pod
logs (`kubectl logs`) - so recipient routing and grouping are checked by
content, not just by counting.

Because the webhook carries a bearer token configured on **both** Alertmanager
and the bridge, the firing run also proves webhook authentication works in the
real chain; `TestClusterWebhookAuthIsEnforced` adds the negative case.

A passing run means a single command turned a freshly created cluster into
delivered e-mails - Prometheus rule evaluation, Alertmanager grouping, recipient
routing, webhook dispatch and resolution, bridge rendering, the OAuth2 flow and
the live `sendMail` call all working together.

## How alerts are defined

Alerts come from a Prometheus rule file embedded in
[`test/cluster/manifests/10-prometheus.yaml`](https://github.com/slauger/alertmanager-graph-bridge/blob/main/test/cluster/manifests/10-prometheus.yaml).
Every rule uses `expr: vector(1)`, which always returns a sample and therefore
fires on the first evaluation - fully deterministic. Three alerts drive the
scenarios:

| Alert | Purpose |
| --- | --- |
| `E2EFiringAlert` | plain firing alert → default recipient |
| `E2EOverrideAlert` | carries an `email_to` label → recipient override |
| `E2EGroupedAlert` | declared twice with one alertname → grouped into one e-mail |

`hack/cluster-e2e.sh` substitutes the real override mailbox for the
`__OVERRIDE_MAIL__` placeholder when it applies the manifest. The resolved
notifications are produced by removing the rules mid-run.

This ConfigMap is the single, easy place to change what the test exercises:
add more `alert:` entries to the `rules.yml` key to drive additional scenarios
through the chain.

## Running it

### Locally (`make cluster-e2e`)

```bash
cp e2e.env.example e2e.env   # then fill it in
make cluster-e2e
```

`make cluster-e2e` builds the orchestrator image and runs
[`hack/cluster-e2e.sh`](https://github.com/slauger/alertmanager-graph-bridge/blob/main/hack/cluster-e2e.sh)
inside it. The script:

1. creates the kind cluster `agb-cluster-e2e`;
2. builds the bridge image and `kind load`s it into the cluster;
3. applies the Prometheus and Alertmanager manifests;
4. installs the bridge with the Helm chart, injecting the secrets from
   `e2e.env`;
5. waits for every workload to become ready and port-forwards their endpoints;
6. runs `go test -tags cluster ./test/cluster/...`;
7. deletes the cluster on exit.

To keep the cluster running for inspection, use `make cluster-e2e-keep` and
clean up afterwards with `kind delete cluster --name agb-cluster-e2e`.

### In GitHub Actions

Store the five variables as repository secrets (see
[e2e-testing.md](e2e-testing.md#in-github-actions)) and run the **cluster-e2e**
workflow (`Actions` tab, `workflow_dispatch`). On the runner the script runs
directly - kind, `kubectl` and `helm` are installed by the workflow - without
the orchestrator container.

## Notes

- The run sends six real e-mails (three firing, three resolved), each with a
  unique `[AGB-CLUSTER <timestamp>]` subject marker printed in the test log.
- `make cluster-e2e` needs access to the host Docker socket
  (`/var/run/docker.sock`); on Docker Desktop this is available by default.
- A full run takes several minutes - creating the kind cluster, pulling the
  Prometheus/Alertmanager images, and waiting for Alertmanager to age the
  alerts out for the resolved-notification phase.
- The cluster is always destroyed on exit unless `--keep` is used, so repeated
  runs start from a clean slate.
