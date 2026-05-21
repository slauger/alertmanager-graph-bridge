output "tenant_id" {
  description = "Microsoft Entra tenant ID (AGB_AZURE_TENANTID)."
  value       = data.azuread_client_config.current.tenant_id
}

output "client_id" {
  description = "Application (client) ID of the bridge registration (AGB_AZURE_CLIENTID)."
  value       = azuread_application.bridge.client_id
}

output "client_secret" {
  description = "Client secret for the bridge registration (AGB_AZURE_CLIENTSECRET)."
  value       = azuread_application_password.bridge.value
  sensitive   = true
}
