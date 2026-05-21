#!/usr/bin/env bash
#
# e2e-set-secrets.sh stores the Azure credentials produced by Terraform, plus
# the mailbox addresses, as GitHub Actions repository secrets so the e2e
# workflow can run. It requires an authenticated gh CLI and a Terraform state
# in ./terraform.
#
# Usage:
#   E2E_MAIL_FROM=monitoring@example.com E2E_MAIL_TO=ops@example.com \
#     ./hack/e2e-set-secrets.sh
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tf_dir="${repo_root}/terraform"

if [ ! -d "${tf_dir}" ]; then
  echo "terraform directory not found: ${tf_dir}" >&2
  exit 1
fi

tenant_id="$(terraform -chdir="${tf_dir}" output -raw tenant_id)"
client_id="$(terraform -chdir="${tf_dir}" output -raw client_id)"
client_secret="$(terraform -chdir="${tf_dir}" output -raw client_secret)"

gh secret set AGB_AZURE_TENANTID --body "${tenant_id}"
gh secret set AGB_AZURE_CLIENTID --body "${client_id}"
gh secret set AGB_AZURE_CLIENTSECRET --body "${client_secret}"
echo "stored AGB_AZURE_* secrets"

if [ -n "${E2E_MAIL_FROM:-}" ]; then
  gh secret set E2E_MAIL_FROM --body "${E2E_MAIL_FROM}"
  echo "stored E2E_MAIL_FROM"
else
  echo "E2E_MAIL_FROM not set; set it manually with: gh secret set E2E_MAIL_FROM"
fi

if [ -n "${E2E_MAIL_TO:-}" ]; then
  gh secret set E2E_MAIL_TO --body "${E2E_MAIL_TO}"
  echo "stored E2E_MAIL_TO"
else
  echo "E2E_MAIL_TO not set; set it manually with: gh secret set E2E_MAIL_TO"
fi
