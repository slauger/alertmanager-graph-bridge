// Package cluster contains the full-chain cluster end-to-end test.
//
// Unlike the test/e2e suite, which posts a hand-crafted webhook payload
// straight at the bridge's HTTP handler, this test exercises the complete
// delivery path inside a real, ephemeral Kubernetes (kind) cluster:
//
//	Prometheus -> Alertmanager -> bridge webhook -> Microsoft Graph -> mailbox
//
// Prometheus generates an always-firing alert, Alertmanager groups it and
// dispatches the webhook (with bearer authentication), and the bridge renders
// the e-mail and calls the live Microsoft Graph API.
//
// The test is guarded by the "cluster" build tag and skips unless the
// CLUSTER_* endpoint environment variables are set, so it never runs as part
// of the default build. It is orchestrated end-to-end by hack/cluster-e2e.sh;
// see docs/cluster-e2e-testing.md.
package cluster
