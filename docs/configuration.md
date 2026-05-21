# Configuration

The bridge is configured from a YAML file (default `config.yaml`, override with
the `-config` flag or `AGB_CONFIG`) and/or environment variables. Environment
variables always take precedence over the file.

## Command-line flags

| Flag | Description |
| --- | --- |
| `-config <path>` | Path to the YAML config file. Default `config.yaml`, or `AGB_CONFIG`. |
| `-version` | Print the version and Go version, then exit. |

## YAML file

```yaml
server:
  port: 8080
  bearerToken: ""        # optional; empty disables authentication
  readTimeout: 10s       # HTTP read timeout
  writeTimeout: 30s      # HTTP write timeout; must exceed mail.sendTimeout
  shutdownGrace: 15s     # graceful shutdown budget

azure:
  tenantId: ""
  clientId: ""
  clientSecret: ""

mail:
  from: "monitoring@example.com"
  to:
    - "ops-team@example.com"
  subjectPrefix: "[alertmanager-graph-bridge]"
  template: modern       # e-mail layout: modern or classic
  saveToSentItems: false
  sendTimeout: 20s       # overall budget for delivering one webhook

log:
  level: info            # debug, info, warn, error
  format: json           # json or text
```

Durations accept Go-style strings such as `10s`, `1m30s` or `500ms`.
`server.writeTimeout` must be greater than `mail.sendTimeout`, otherwise the
HTTP write deadline could fire while a slow Microsoft Graph call is still in
flight; this is enforced at startup.

## Environment variables

Every value can be supplied or overridden through an `AGB_`-prefixed variable.

| Variable | Maps to | Notes |
| --- | --- | --- |
| `AGB_CONFIG` | config file path | Default `config.yaml` |
| `AGB_SERVER_PORT` | `server.port` | Integer |
| `AGB_SERVER_BEARERTOKEN` | `server.bearerToken` | Empty disables auth |
| `AGB_SERVER_READTIMEOUT` | `server.readTimeout` | Duration |
| `AGB_SERVER_WRITETIMEOUT` | `server.writeTimeout` | Duration |
| `AGB_SERVER_SHUTDOWNGRACE` | `server.shutdownGrace` | Duration |
| `AGB_MAIL_SENDTIMEOUT` | `mail.sendTimeout` | Duration |
| `AGB_AZURE_TENANTID` | `azure.tenantId` | Required |
| `AGB_AZURE_CLIENTID` | `azure.clientId` | Required |
| `AGB_AZURE_CLIENTSECRET` | `azure.clientSecret` | Required; prefer env over file |
| `AGB_MAIL_FROM` | `mail.from` | Required, valid address |
| `AGB_MAIL_TO` | `mail.to` | Comma-separated list |
| `AGB_MAIL_SUBJECTPREFIX` | `mail.subjectPrefix` | |
| `AGB_MAIL_TEMPLATE` | `mail.template` | `modern` or `classic` |
| `AGB_MAIL_SAVETOSENTITEMS` | `mail.saveToSentItems` | `true` or `false` |
| `AGB_LOG_LEVEL` | `log.level` | |
| `AGB_LOG_FORMAT` | `log.format` | |

## Validation

On startup the configuration is validated. The process exits with an error if:

- `server.port` is outside `1-65535`,
- any of `azure.tenantId`, `azure.clientId`, `azure.clientSecret` is empty,
- `mail.from` is missing or not a valid e-mail address,
- `mail.to` is empty or contains an invalid address,
- `mail.template` is not `modern` or `classic`,
- `log.level` or `log.format` is not one of the allowed values,
- `server.writeTimeout` is not greater than `mail.sendTimeout`.

## Per-alert recipients

By default every alert goes to `mail.to`. An individual alert can override the
recipients with an `email_to` label containing a comma-separated address list:

```yaml
- alert: DatabaseDown
  labels:
    severity: critical
    email_to: "dba-team@example.com,oncall@example.com"
```

Alerts in a single webhook that resolve to different recipient sets are sent as
separate grouped e-mails.

Addresses in an `email_to` label that are not valid e-mail addresses are
dropped. If that leaves an alert with no explicit recipients, it falls back to
the configured `mail.to` list, so a single typo cannot block delivery.

## E-mail templates

`mail.template` selects the HTML layout of the generated e-mails:

| Value | Description |
| --- | --- |
| `modern` | The default. A card-based layout with status badges, label chips and formatted timestamps. |
| `classic` | The look of the stock Prometheus Alertmanager e-mail: firing and resolved sections with plain label/annotation lists. |

Both templates carry the same information and the same product branding; only
the visual style differs.
