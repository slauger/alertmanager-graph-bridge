terraform {
  required_version = ">= 1.6"

  required_providers {
    azuread = {
      source  = "hashicorp/azuread"
      version = "~> 3.0"
    }
  }
}

# Authentication is taken from the environment: run `az login` first, or set
# ARM_TENANT_ID / ARM_CLIENT_ID / ARM_CLIENT_SECRET for a service principal.
provider "azuread" {}

data "azuread_client_config" "current" {}

# Well-known Microsoft first-party application IDs (Microsoft Graph).
data "azuread_application_published_app_ids" "well_known" {}

# The Microsoft Graph service principal that already exists in every tenant.
resource "azuread_service_principal" "msgraph" {
  client_id    = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]
  use_existing = true
}

# Application registration used by alertmanager-graph-bridge.
resource "azuread_application" "bridge" {
  display_name = var.application_name
  owners       = [data.azuread_client_config.current.object_id]

  required_resource_access {
    resource_app_id = data.azuread_application_published_app_ids.well_known.result["MicrosoftGraph"]

    # Mail.Send as an application permission (type "Role").
    resource_access {
      id   = azuread_service_principal.msgraph.app_role_ids["Mail.Send"]
      type = "Role"
    }
  }
}

# Service principal for the application.
resource "azuread_service_principal" "bridge" {
  client_id = azuread_application.bridge.client_id
  owners    = [data.azuread_client_config.current.object_id]
}

# Client secret the bridge uses for the OAuth2 client-credentials flow.
resource "azuread_application_password" "bridge" {
  application_id    = azuread_application.bridge.id
  display_name      = "alertmanager-graph-bridge"
  end_date_relative = var.secret_validity
}

# Grant admin consent for the Mail.Send application permission by assigning the
# Microsoft Graph app role to the bridge service principal.
resource "azuread_app_role_assignment" "mail_send" {
  app_role_id         = azuread_service_principal.msgraph.app_role_ids["Mail.Send"]
  principal_object_id = azuread_service_principal.bridge.object_id
  resource_object_id  = azuread_service_principal.msgraph.object_id
}
