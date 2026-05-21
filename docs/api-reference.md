# API Reference

The server exposes four HTTP endpoints.

## POST /api/v1/alerts

Receives an Alertmanager webhook notification (schema version 4).

- **Content-Type:** `application/json`
- **Authentication:** `Authorization: Bearer <token>` when `server.bearerToken`
  is configured; otherwise unauthenticated.
- **Request body:** the standard Alertmanager webhook payload.

Responses:

| Status | Meaning |
| --- | --- |
| `200 OK` | All alert groups were delivered (or the payload had no alerts). |
| `400 Bad Request` | The body was not valid or the webhook version unsupported. |
| `401 Unauthorized` | Bearer token missing or incorrect. |
| `502 Bad Gateway` | At least one alert group failed to send. |

Example:

```bash
curl -X POST localhost:8080/api/v1/alerts \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d @webhook.json
```

## GET /healthz

Liveness probe. Always returns `200 OK` with the body `ok` while the process is
running.

## GET /readyz

Readiness probe. Returns `200 OK` (`ready`) once the server has started, and
`503 Service Unavailable` (`not ready`) during startup and graceful shutdown.

## GET /metrics

Prometheus metrics in the text exposition format.

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `agb_webhook_requests_total` | counter | `outcome` | Webhook requests by outcome (`ok`, `empty`, `bad_request`, `unauthorized`, `send_failed`). |
| `agb_webhook_request_duration_seconds` | histogram | - | Latency of webhook request handling. |
| `agb_mails_sent_total` | counter | - | E-mails successfully sent via Graph. |
| `agb_mail_send_errors_total` | counter | `reason` | Failed sends (`graph_4xx`, `graph_429`, `graph_5xx`, `transport`, `render`). |
| `agb_mail_send_duration_seconds` | histogram | - | Latency of `sendMail` calls. |
| `agb_panics_recovered_total` | counter | - | Panics recovered by the HTTP middleware. |
| `agb_build_info` | gauge | `version`, `goversion` | Build information (always `1`). |

The standard Go runtime and process collectors are also exported.
