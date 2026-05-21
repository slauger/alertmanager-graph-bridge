// Package e2e contains end-to-end tests that exercise alertmanager-graph-bridge
// against the live Microsoft Graph API.
//
// The tests are compiled only under the "e2e" build tag and are skipped unless
// the required AGB_AZURE_* and E2E_MAIL_* environment variables are set. See
// e2e_test.go and docs/e2e-testing.md.
package e2e
