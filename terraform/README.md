# Terraform: Azure test resources

This module provisions the Microsoft Entra resources needed to run the
end-to-end tests against the live Microsoft Graph API:

- an application registration,
- a service principal,
- a client secret,
- the `Mail.Send` Microsoft Graph **application** permission,
- admin consent for that permission.

## Prerequisites

- Terraform >= 1.6.
- The [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/), authenticated
  with `az login`.
- A directory role that allows creating app registrations and granting admin
  consent (Application Administrator + Privileged Role Administrator, or
  Global Administrator).
- An existing mailbox in the tenant to send test mail from.

## Usage

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars   # optional, to override defaults
terraform init
terraform apply
```

After `apply`, read the outputs:

```bash
terraform output -raw tenant_id
terraform output -raw client_id
terraform output -raw client_secret
```

These map to `AGB_AZURE_TENANTID`, `AGB_AZURE_CLIENTID` and
`AGB_AZURE_CLIENTSECRET`. See [`docs/e2e-testing.md`](../docs/e2e-testing.md)
for how to run the end-to-end tests with them.

## Cleanup

```bash
terraform destroy
```

## Notes

- Permission grants can take a minute or two to propagate before the first
  token request succeeds.
- `Mail.Send` as an application permission lets the app send as any mailbox in
  the tenant. To restrict it to a single mailbox, create an Exchange Online
  [application access policy](https://learn.microsoft.com/en-us/graph/auth-limit-mailbox-access)
  with `New-ApplicationAccessPolicy` (Exchange Online PowerShell; outside the
  scope of this module).
