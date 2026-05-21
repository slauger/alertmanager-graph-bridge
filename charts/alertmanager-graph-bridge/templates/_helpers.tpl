{{/* Chart name. */}}
{{- define "alertmanager-graph-bridge.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified app name. */}}
{{- define "alertmanager-graph-bridge.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Chart label value. */}}
{{- define "alertmanager-graph-bridge.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels. */}}
{{- define "alertmanager-graph-bridge.labels" -}}
helm.sh/chart: {{ include "alertmanager-graph-bridge.chart" . }}
{{ include "alertmanager-graph-bridge.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* Selector labels. */}}
{{- define "alertmanager-graph-bridge.selectorLabels" -}}
app.kubernetes.io/name: {{ include "alertmanager-graph-bridge.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Service account name. */}}
{{- define "alertmanager-graph-bridge.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "alertmanager-graph-bridge.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* Name of the secret holding the Azure credentials. */}}
{{- define "alertmanager-graph-bridge.secretName" -}}
{{- default (include "alertmanager-graph-bridge.fullname" .) .Values.existingSecret -}}
{{- end -}}

{{/* Container image reference. The tag defaults to the chart appVersion, or
"latest" when the chart still carries the 0.0.0 development placeholder. */}}
{{- define "alertmanager-graph-bridge.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if or (not $tag) (eq $tag "0.0.0") -}}
{{- $tag = "latest" -}}
{{- end -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
