{{/*
Expand the name of the chart.
*/}}
{{- define "udash.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "udash.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "udash.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "udash.labels" -}}
helm.sh/chart: {{ include "udash.chart" . }}
{{ include "udash.selectorLabels.front" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "udash.selectorLabels.agent" -}}
app.kubernetes.io/name: {{ include "udash.name" . }}-agent
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
{{- define "udash.selectorLabels.server" -}}
app.kubernetes.io/name: {{ include "udash.name" . }}-server
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
{{- define "udash.selectorLabels.front" -}}
app.kubernetes.io/name: {{ include "udash.name" . }}-front
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "udash.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "udash.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the configmap to use for configuration
*/}}
{{- define "udash.configMapName" -}}
{{- default (include "udash.fullname" .) .Values.configMap.name }}
{{- end }}

{{/*
Create the name of the secrets use by udash agents
*/}}
{{- define "udash.secretName" -}}
{{- default (include "udash.fullname" .) .Values.secrets.name }}
{{- end }}
