{{/*
Expand chart name.
*/}}
{{- define "otel-stack.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "otel-stack.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "otel-stack.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
environment: {{ .Values.global.environment }}
cluster: {{ .Values.global.clusterName }}
{{- end }}

{{/*
Namespace helper.
*/}}
{{- define "otel-stack.namespace" -}}
{{ .Values.global.namespace }}
{{- end }}
