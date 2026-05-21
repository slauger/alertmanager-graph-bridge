// Package branding holds the user-visible product name.
//
// ProductName is the single source of truth for the brand. It is compiled
// into the binary, so every built image carries a fixed name. To rebrand,
// change the value below and rebuild, or override it at build time without
// touching source:
//
//	go build -ldflags "-X 'github.com/slauger/alertmanager-graph-bridge/internal/branding.ProductName=Acme Alerts'"
//
// The Makefile exposes the same override through the PRODUCT_NAME variable,
// and the container build through the PRODUCT_NAME build argument.
package branding

// ProductName is the user-visible product name shown in alert e-mails, the
// startup banner and the --version output. It is intentionally a var so it
// can be overridden at link time; treat it as read-only at runtime.
var ProductName = "alertmanager-graph-bridge"
