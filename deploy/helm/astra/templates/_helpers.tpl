{{/*
_helpers.tpl — Shared template functions for the Astra Helm chart
*/}}

{{/*
Expand the name of the chart
*/}}
{{- define "astra.name" -}}
{{- .Chart.Name | default .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name (release + chart)
*/}}
{{- define "astra.fullname" -}}
{{- $name := include "astra.name" . }}
{{- if .Values.fullnameOverride }}
  {{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
  {{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the label selector of the deployment.
*/}}
{{- define "astra.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels — applied to all resources
*/}}
{{- define "astra.labels" -}}
app.kubernetes.io/name: {{ include "astra.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: {{ .Values.app.name | default (include "astra.name" .) }}
helm.sh/chart: {{ include "astra.chart" . }}
{{- with .Values.global.labels }}
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
Selector labels — applied to the Deployment and Service
*/}}
{{- define "astra.selectorLabels" -}}
app.kubernetes.io/name: {{ include "astra.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service Account name
*/}}
{{- define "astra.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
  {{- default (include "astra.fullname" .) .Values.serviceAccount.name }}
{{- else }}
  {{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image full URL with registry
*/}}
{{- define "astra.image" -}}
{{- $registry := .Values.global.imageRegistry -}}
{{- $repo := .Values.image.repository -}}
{{- printf "%s/%s" $registry $repo | trimSuffix "/" }}
{{- end }}
