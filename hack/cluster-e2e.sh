#!/usr/bin/env bash
#
# cluster-e2e.sh orchestrates the full-chain cluster end-to-end test.
#
# It creates an ephemeral kind cluster, builds and loads the bridge image,
# deploys Prometheus + Alertmanager + the bridge (via the Helm chart), waits
# for everything to become ready, port-forwards the service endpoints, and runs
# the `cluster`-tagged Go test that asserts an alert travels the whole chain:
#
#   Prometheus -> Alertmanager -> bridge webhook -> Microsoft Graph -> mailbox
#
# It runs both inside the containerized orchestrator (`make cluster-e2e`) and
# directly on a GitHub Actions runner. The cluster is destroyed on exit unless
# `--keep` is passed.
#
# Required environment (see e2e.env.example):
#   AGB_AZURE_TENANTID  AGB_AZURE_CLIENTID  AGB_AZURE_CLIENTSECRET
#   E2E_MAIL_FROM       E2E_MAIL_TO
set -euo pipefail

CLUSTER_NAME="agb-cluster-e2e"
NAMESPACE="agb-e2e"
RELEASE="agb"
IMAGE="alertmanager-graph-bridge:cluster-e2e"
BRIDGE_DEPLOY="${RELEASE}-alertmanager-graph-bridge"
# Shared by the Alertmanager webhook config and the bridge; exercises auth.
BEARER_TOKEN="cluster-e2e-bearer-token"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

KEEP=0
[ "${1:-}" = "--keep" ] && KEEP=1

# --- 1. Required environment ------------------------------------------------
missing=0
for name in AGB_AZURE_TENANTID AGB_AZURE_CLIENTID AGB_AZURE_CLIENTSECRET \
            E2E_MAIL_FROM E2E_MAIL_TO; do
  if [ -z "${!name:-}" ]; then
    echo "error: required environment variable ${name} is not set" >&2
    missing=1
  fi
done
if [ "$missing" -ne 0 ]; then
  echo "Populate e2e.env (see e2e.env.example) and retry." >&2
  exit 1
fi

SUBJECT_MARKER="[AGB-CLUSTER $(date -u +%Y%m%dT%H%M%SZ)]"
echo "==> Cluster e2e run; e-mail subject marker: ${SUBJECT_MARKER}"

export KUBECONFIG="${REPO_ROOT}/test/cluster/kubeconfig"

PF_PIDS=()
cleanup() {
  for pid in "${PF_PIDS[@]:-}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
  if [ "$KEEP" -eq 1 ]; then
    echo "==> --keep set; leaving cluster '${CLUSTER_NAME}' running"
    echo "    KUBECONFIG=${KUBECONFIG}"
    echo "    Tear down with: kind delete cluster --name ${CLUSTER_NAME}"
    return
  fi
  echo "==> Tearing down kind cluster '${CLUSTER_NAME}'"
  kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
  rm -f "$KUBECONFIG"
}
trap cleanup EXIT

# --- 2. Fresh kind cluster --------------------------------------------------
echo "==> Creating kind cluster '${CLUSTER_NAME}'"
kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
kind create cluster --name "$CLUSTER_NAME" \
  --config test/cluster/kind-config.yaml --wait 120s

# --- 3. kubeconfig: containerized orchestrator vs. host runner --------------
if [ -f /.dockerenv ]; then
  # Inside the orchestrator container the kind API server is not on localhost.
  # Join the 'kind' Docker network and use the network-internal kubeconfig.
  echo "==> Running inside a container; joining the 'kind' Docker network"
  docker network connect kind "$(hostname)" >/dev/null 2>&1 || true
  kind get kubeconfig --name "$CLUSTER_NAME" --internal > "$KUBECONFIG"
else
  kind get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG"
fi

# --- 4. Build and load the bridge image -------------------------------------
echo "==> Building bridge image ${IMAGE}"
docker build -f images/alertmanager-graph-bridge/Containerfile -t "$IMAGE" .
echo "==> Loading ${IMAGE} into the cluster"
kind load docker-image "$IMAGE" --name "$CLUSTER_NAME"

# --- 5. Prometheus + Alertmanager -------------------------------------------
# The E2EOverrideAlert rule routes via an email_to label; substitute the real
# mailbox for the placeholder. The override is sent to E2E_MAIL_FROM, a known
# valid mailbox in the tenant.
echo "==> Deploying Prometheus and Alertmanager"
kubectl apply -f test/cluster/manifests/00-namespace.yaml
kubectl apply -f test/cluster/manifests/20-alertmanager.yaml
sed "s|__OVERRIDE_MAIL__|${E2E_MAIL_FROM}|g" test/cluster/manifests/10-prometheus.yaml \
  | kubectl apply -f -

# --- 6. Bridge via the Helm chart -------------------------------------------
echo "==> Installing the bridge via Helm"
helm upgrade --install "$RELEASE" charts/alertmanager-graph-bridge \
  --namespace "$NAMESPACE" --create-namespace \
  -f test/cluster/bridge-values.yaml \
  --set "config.azure.tenantId=${AGB_AZURE_TENANTID}" \
  --set "config.azure.clientId=${AGB_AZURE_CLIENTID}" \
  --set "config.mail.from=${E2E_MAIL_FROM}" \
  --set "config.mail.to[0]=${E2E_MAIL_TO}" \
  --set-string "config.mail.subjectPrefix=${SUBJECT_MARKER}" \
  --set "secret.clientSecret=${AGB_AZURE_CLIENTSECRET}" \
  --set "secret.bearerToken=${BEARER_TOKEN}" \
  --wait --timeout 180s

# --- 7. Wait for the monitoring stack ---------------------------------------
echo "==> Waiting for all workloads to become ready"
kubectl -n "$NAMESPACE" rollout status deploy/prometheus --timeout=120s
kubectl -n "$NAMESPACE" rollout status deploy/alertmanager --timeout=120s
kubectl -n "$NAMESPACE" rollout status "deploy/${BRIDGE_DEPLOY}" --timeout=120s

# --- 8. Port-forward the service endpoints to localhost ---------------------
echo "==> Port-forwarding service endpoints"
kubectl -n "$NAMESPACE" port-forward svc/prometheus 9090:9090 >/dev/null 2>&1 &
PF_PIDS+=($!)
kubectl -n "$NAMESPACE" port-forward svc/alertmanager 9093:9093 >/dev/null 2>&1 &
PF_PIDS+=($!)
kubectl -n "$NAMESPACE" port-forward "svc/${BRIDGE_DEPLOY}" 8080:8080 >/dev/null 2>&1 &
PF_PIDS+=($!)
sleep 5

# --- 9. Run the assertion suite ---------------------------------------------
# The resolved-notification test removes the Prometheus rules and waits for
# Alertmanager to age the alerts out, so the suite needs a generous timeout.
echo "==> Running the cluster e2e assertions"
CLUSTER_PROMETHEUS_URL="http://127.0.0.1:9090" \
CLUSTER_ALERTMANAGER_URL="http://127.0.0.1:9093" \
CLUSTER_BRIDGE_URL="http://127.0.0.1:8080" \
CLUSTER_SUBJECT_MARKER="${SUBJECT_MARKER}" \
CLUSTER_NAMESPACE="${NAMESPACE}" \
CLUSTER_BRIDGE_DEPLOY="${BRIDGE_DEPLOY}" \
CLUSTER_OVERRIDE_MAIL="${E2E_MAIL_FROM}" \
  go test -tags cluster -v -count=1 -timeout 900s ./test/cluster/...

echo "==> Cluster e2e run completed successfully"
