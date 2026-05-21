# End-to-End Testing

The end-to-end (e2e) tests exercise the full bridge against the **live
Microsoft Graph API**: real OAuth2 token acquisition, the `Mail.Send`
permission, and real `sendMail` calls that deliver actual e-mails. They
complement the unit and integration tests, which mock Graph entirely.

## TL;DR

```bash
az login                                          # as a tenant admin
cd terraform && terraform init && terraform apply  # create the Azure app
cd .. && cp e2e.env.example e2e.env                # fill in outputs + mailboxes
make e2e-build && make e2e-run                     # run the suite in a container
```

## What you need

### Microsoft 365 / Entra

| Requirement | Notes |
| --- | --- |
| A Microsoft Entra (Azure AD) tenant | No Azure **subscription** is needed - only Entra objects are created, nothing billable. |
| At least one licensed mailbox | Used for `E2E_MAIL_FROM` and `E2E_MAIL_TO`. They may be the **same** address (send-to-self), so one mailbox is enough. It must be a real Exchange Online mailbox. |
| An admin account | Must be able to create an app registration **and** grant admin consent. Simplest: **Global Administrator**; otherwise *Application Administrator* + *Privileged Role Administrator*. |

### Local tooling

| Tool | Purpose | Required |
| --- | --- | --- |
| Azure CLI (`az`) | `az login` - Terraform uses this sign-in | yes |
| Terraform >= 1.6 | Provisions the app registration, secret and permission | yes |
| Docker | Builds and runs the e2e container | for the container path |
| Go 1.26 | Alternative: `make e2e-local` without a container | only if no Docker |
| `gh` CLI | Only to push the GitHub Actions secrets via the helper | optional |

### Cost

Effectively zero. Microsoft Graph API calls and the Entra app registration are
free; there is no Azure consumption billing. The only possible real cost is an
M365 license if you create a mailbox solely for testing (existing mailboxes
incur nothing). Each run sends five real e-mails, far below the Graph sending
limits.

## What the tests cover

Each test wires the real Graph client and HTTP server (exactly as the binary
does) and posts an Alertmanager webhook through it:

- `TestE2EFiringAlertAccepted` - a firing alert is sent.
- `TestE2EResolvedAlertAccepted` - a resolved alert is sent.
- `TestE2EGroupedAlertsAccepted` - several alerts are grouped into one e-mail.
- `TestE2ERecipientOverrideFanOut` - one alert is routed by its `email_to`
  label and one by the default recipient, exercising fan-out into two e-mails.
- `TestE2EBearerAuthentication` - an unauthenticated request is rejected; an
  authenticated one is sent.
- `TestE2EInvalidSenderReturns502` - a non-existent sender mailbox makes Graph
  reject the request, and the bridge returns 502.
- `TestE2EInvalidCredentialsReturn502` - a wrong client secret makes the OAuth2
  token request fail, and the bridge returns 502.

The five delivery scenarios (firing, resolved, grouped, fan-out, authenticated)
are split across the two e-mail templates, so a full run sends five e-mails
that together exercise both the `modern` and the `classic` layout. The template
name is woven into the subject prefix (`[AGB-E2E modern]` / `[AGB-E2E classic]`)
to tell the resulting e-mails apart in the mailbox.

A passing run means the OAuth2 client-credentials flow, the `Mail.Send`
permission, the request schema, the sender mailbox and the error-handling path
all work against the live tenant. "Accepted" means Microsoft Graph returned
`202`; Exchange Online still performs delivery asynchronously. The successful
e-mails arrive in the mailboxes with a timestamped marker in the subject.

## 1. Provision the Azure resources

The [`terraform/`](https://github.com/slauger/alertmanager-graph-bridge/tree/main/terraform)
module creates the app registration, service principal, client secret and the
`Mail.Send` permission with admin consent.

```bash
az login
cd terraform
terraform init
terraform apply
```

See [`terraform/README.md`](https://github.com/slauger/alertmanager-graph-bridge/blob/main/terraform/README.md)
for details.

## 2. Configuration

The tests read five environment variables:

| Variable | Source |
| --- | --- |
| `AGB_AZURE_TENANTID` | `terraform output -raw tenant_id` |
| `AGB_AZURE_CLIENTID` | `terraform output -raw client_id` |
| `AGB_AZURE_CLIENTSECRET` | `terraform output -raw client_secret` |
| `E2E_MAIL_FROM` | an existing mailbox to send from |
| `E2E_MAIL_TO` | an existing mailbox to receive the test e-mails |

If any variable is unset the tests skip, so they are a safe no-op without
credentials.

## 3. Run the tests

### In a container (recommended)

```bash
cp e2e.env.example e2e.env   # then fill it in
make e2e-build
make e2e-run
```

`make e2e-build` builds the e2e image from
`images/alertmanager-graph-bridge-e2e/Containerfile`; `make e2e-run` runs it
with the values from `e2e.env`.

### Locally with the Go toolchain

```bash
export AGB_AZURE_TENANTID=... AGB_AZURE_CLIENTID=... AGB_AZURE_CLIENTSECRET=...
export E2E_MAIL_FROM=monitoring@example.com E2E_MAIL_TO=ops-team@example.com
make e2e-local
```

### In GitHub Actions

Store the five variables as repository secrets, then run the **e2e** workflow
(`Actions` tab, `workflow_dispatch`). The workflow builds the e2e image and runs
it against the live tenant.

The Azure secrets can be set straight from the Terraform outputs:

```bash
E2E_MAIL_FROM=monitoring@example.com E2E_MAIL_TO=ops-team@example.com \
  ./hack/e2e-set-secrets.sh
```

## 4. Cleanup

```bash
cd terraform && terraform destroy
```

## Notes

- Allow a minute or two after `terraform apply` for the permission grant to
  propagate before the first run.
- Each run sends five real e-mails to `E2E_MAIL_TO`, split across the `modern`
  and `classic` templates.
- The runtime environment needs outbound access to `login.microsoftonline.com`
  and `graph.microsoft.com`.
- `Mail.Send` as an application permission lets the app send as any mailbox in
  the tenant; restrict it with an Exchange Online application access policy if
  required (see `terraform/README.md`).
