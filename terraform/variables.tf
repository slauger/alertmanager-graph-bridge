variable "application_name" {
  description = "Display name of the Microsoft Entra application registration."
  type        = string
  default     = "alertmanager-graph-bridge-e2e"
}

variable "secret_validity" {
  description = "Lifetime of the generated client secret, as a Go-style duration (e.g. 4320h is 180 days)."
  type        = string
  default     = "4320h"
}
