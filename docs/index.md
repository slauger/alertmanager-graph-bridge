# alertmanager-graph-bridge

**alertmanager-graph-bridge** is a lightweight
HTTP server that bridges **Prometheus Alertmanager** webhook notifications to
the **Microsoft Graph API**. It receives Alertmanager webhook payloads, renders
them into readable HTML e-mails and sends them through `sendMail`,
authenticating with the OAuth2 client-credentials flow.

## Why

Alertmanager has no built-in support for sending mail through Microsoft 365 /
Exchange Online via the Graph API. Many organisations have disabled SMTP AUTH
and require applications to use Graph with OAuth2 instead. This bridge fills
that gap with a single, small Go binary.

## Features

- Implements the Alertmanager webhook API (`POST /api/v1/alerts`).
- Optional bearer-token authentication for incoming requests.
- Health (`/healthz`), readiness (`/readyz`) and Prometheus metrics
  (`/metrics`) endpoints.
- Microsoft Graph `sendMail` integration with OAuth2 client credentials and
  automatic token caching / refresh.
- HTML e-mail rendering that groups multiple alerts into one message, with a
  selectable `modern` or `classic` template.
- Per-alert recipient overrides via the `email_to` label.
- Configuration through a YAML file and/or `AGB_`-prefixed environment
  variables.
- Structured logging (`slog`) in JSON or text format.

## Next steps

- [Getting Started](getting-started.md) - install and run the bridge.
- [Configuration](configuration.md) - all configuration options.
- [Architecture](architecture.md) - how the pieces fit together.
- [API Reference](api-reference.md) - the HTTP endpoints and metrics.
- [Development](development.md) - build and test locally.
- [End-to-End Testing](e2e-testing.md) - testing against the live Microsoft Graph API.
- [Quality Criteria](quality.md) - the bar every change is held to.
