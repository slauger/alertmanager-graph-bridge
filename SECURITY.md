# Security Policy

## Supported versions

The latest released version receives security fixes. Older versions are not
maintained.

## Reporting a vulnerability

Please report security issues privately rather than opening a public issue.
Use GitHub's [private vulnerability reporting](https://github.com/slauger/alertmanager-graph-bridge/security/advisories/new)
for this repository.

Include a description of the issue, steps to reproduce, and the affected
version. You can expect an initial response within a few business days.

## Handling of secrets

- The Azure client secret and the webhook bearer token are read from the
  configuration or `AGB_`-prefixed environment variables and are never written
  to logs.
- In Kubernetes, store these values in a `Secret`; the Helm chart can either
  create one or reference an existing secret via `existingSecret`.
- OAuth2 access tokens are held in memory only and are never persisted.

## Hardening notes

- Run the container as the provided non-root user (`USER 1001:0`) with a
  read-only root filesystem, as the Helm chart does by default.
- Configure `server.bearerToken` so the webhook endpoint is authenticated.
- Restrict which mailboxes the Azure app may send as with an
  [application access policy](https://learn.microsoft.com/en-us/graph/auth-limit-mailbox-access).
