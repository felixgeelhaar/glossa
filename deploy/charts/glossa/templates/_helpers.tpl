{{/*
Naming helpers. Keep names predictable so cluster-wide tooling
(e.g. `kubectl get deploy glossa-api`) just works.
*/}}

{{- define "glossa.fullname" -}}
{{- default .Release.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "glossa.labels" -}}
app.kubernetes.io/name: glossa
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "glossa.selectorLabels" -}}
app.kubernetes.io/name: glossa
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "glossa.api.image" -}}
{{- $tag := .Values.api.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.api.image.repository $tag -}}
{{- end -}}

{{- define "glossa.admin.image" -}}
{{- $tag := .Values.admin.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.admin.image.repository $tag -}}
{{- end -}}

{{- define "glossa.postgres.image" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.postgres.image.repository .Values.postgres.image.tag -}}
{{- end -}}

{{- define "glossa.secretName" -}}
{{- .Values.secrets.existingSecret | default "glossa-app" -}}
{{- end -}}
