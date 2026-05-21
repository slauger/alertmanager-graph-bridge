# Getting Started

## Prerequisites

- A Microsoft Entra (Azure AD) tenant.
- An app registration with the **`Mail.Send`** application permission, granted
  admin consent.
- A mailbox to send from (a real user or a shared mailbox).

## Azure app registration

1. In the Entra admin center, create an **App registration**.
2. Note the **Directory (tenant) ID** and **Application (client) ID**.
3. Under **Certificates & secrets**, create a **client secret** and copy its
   value.
4. Under **API permissions**, add **Microsoft Graph -> Application permissions
   -> `Mail.Send`** and click **Grant admin consent**.

!!! note
    `Mail.Send` as an application permission allows the app to send mail as any
    mailbox in the tenant. Restrict this with an
    [application access policy](https://learn.microsoft.com/en-us/graph/auth-limit-mailbox-access)
    if required.

## Install with Helm

```bash
helm install alertmanager-graph-bridge \
  oci://ghcr.io/slauger/charts/alertmanager-graph-bridge \
  --namespace monitoring --create-namespace \
  --set config.azure.tenantId=<TENANT_ID> \
  --set config.azure.clientId=<CLIENT_ID> \
  --set secret.clientSecret=<CLIENT_SECRET> \
  --set config.mail.from=monitoring@example.com \
  --set 'config.mail.to={ops-team@example.com}'
```

To reference a pre-existing Secret instead of passing the value inline:

```bash
helm install alertmanager-graph-bridge \
  oci://ghcr.io/slauger/charts/alertmanager-graph-bridge \
  --namespace monitoring --create-namespace \
  --set config.azure.tenantId=<TENANT_ID> \
  --set config.azure.clientId=<CLIENT_ID> \
  --set existingSecret=my-azure-secret \
  --set config.mail.from=monitoring@example.com \
  --set 'config.mail.to={ops-team@example.com}'
```

The existing Secret must contain at least a `clientSecret` key and optionally a
`bearerToken` key.

## Run with the container image

```bash
docker run --rm -p 8080:8080 \
  -e AGB_AZURE_TENANTID=<TENANT_ID> \
  -e AGB_AZURE_CLIENTID=<CLIENT_ID> \
  -e AGB_AZURE_CLIENTSECRET=<CLIENT_SECRET> \
  -e AGB_MAIL_FROM=monitoring@example.com \
  -e AGB_MAIL_TO=ops-team@example.com \
  ghcr.io/slauger/alertmanager-graph-bridge:latest
```

## Point Alertmanager at the bridge

```yaml
receivers:
  - name: graph-bridge
    webhook_configs:
      - url: http://alertmanager-graph-bridge.monitoring.svc:8080/api/v1/alerts
        # When a bearer token is configured on the bridge:
        http_config:
          authorization:
            type: Bearer
            credentials: <BEARER_TOKEN>

route:
  receiver: graph-bridge
```

## Verify

```bash
curl -s localhost:8080/healthz   # -> ok
curl -s localhost:8080/readyz    # -> ready
curl -s localhost:8080/metrics   # -> Prometheus metrics
```
