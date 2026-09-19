{{/* ── Names and labels ─────────────────────────────────────────── */}}

{{- define "gp.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Prefix of every resource name. */}}
{{- define "gp.fullname" -}}
{{- default .Release.Name .Values.fullnameOverride | trunc 50 | trimSuffix "-" -}}
{{- end -}}

{{/* Name of a component's resources: (dict "root" $ "component" "server"). */}}
{{- define "gp.componentName" -}}
{{- printf "%s-%s" (include "gp.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "gp.labels" -}}
app.kubernetes.io/name: {{ include "gp.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: glossa
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* (dict "root" $ "component" "server") */}}
{{- define "gp.componentLabels" -}}
{{ include "gp.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* (dict "root" $ "component" "server") */}}
{{- define "gp.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gp.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* ── Images ───────────────────────────────────────────────────── */}}

{{/* (dict "root" $ "image" .Values.server.image) */}}
{{- define "gp.image" -}}
{{- $tag := .image.tag | default .root.Chart.AppVersion -}}
{{- $ref := printf "%s/%s:%s" .root.Values.image.registry .image.repository $tag -}}
{{- if .image.digest -}}
{{- $ref = printf "%s@%s" $ref .image.digest -}}
{{- end -}}
{{- $ref -}}
{{- end -}}

{{/* ── Hosts and URLs ───────────────────────────────────────────── */}}

{{- define "gp.host.studio" -}}
{{- required "hosts.studio is required (e.g. app.<domain>)" .Values.hosts.studio -}}
{{- end -}}

{{- define "gp.host.api" -}}
{{- required "hosts.api is required (e.g. api.<domain>)" .Values.hosts.api -}}
{{- end -}}

{{- define "gp.host.cdn" -}}
{{- required "hosts.cdn is required (e.g. cdn.<domain>)" .Values.hosts.cdn -}}
{{- end -}}

{{- define "gp.studioUrl" -}}
{{- printf "https://%s" (include "gp.host.studio" .) -}}
{{- end -}}

{{/* ── Pod hardening (PodSecurity "restricted") ─────────────────── */}}

{{/* Pod securityContext; argument: the numeric uid/gid of the image. */}}
{{- define "gp.podSecurityContext" -}}
runAsNonRoot: true
runAsUser: {{ . }}
runAsGroup: {{ . }}
fsGroup: {{ . }}
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{- define "gp.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
runAsNonRoot: true
capabilities:
  drop: [ALL]
seccompProfile:
  type: RuntimeDefault
{{- end -}}

{{/* Default soft spread of a component's pods across nodes. */}}
{{- define "gp.topologySpread" -}}
{{- if .values.topologySpreadConstraints -}}
{{ toYaml .values.topologySpreadConstraints }}
{{- else -}}
- maxSkew: 1
  topologyKey: kubernetes.io/hostname
  whenUnsatisfiable: ScheduleAnyway
  labelSelector:
    matchLabels:
      {{- include "gp.selectorLabels" (dict "root" .root "component" .component) | nindent 6 }}
{{- end -}}
{{- end -}}

{{/* nodeSelector, tolerations, affinity, pull secrets of a component. */}}
{{- define "gp.scheduling" -}}
{{- with .root.Values.image.pullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .values.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .values.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .values.affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end -}}

{{/* ── Mail ─────────────────────────────────────────────────────── */}}

{{/* The effective GLOSSA_MAIL_DRIVER: mail.driver, else smtp when an SMTP
     server is set, else "" (the variable is left unset). */}}
{{- define "gp.mailDriver" -}}
{{- .Values.mail.driver | default (ternary "smtp" "" (not (empty .Values.mail.smtp.addr))) -}}
{{- end -}}

{{/* ── Secret references ────────────────────────────────────────── */}}

{{/* (dict "name" "ENV_NAME" "secret" "secret-name" "key" "key") */}}
{{- define "gp.secretEnv" -}}
- name: {{ .name }}
  valueFrom:
    secretKeyRef:
      name: {{ .secret }}
      key: {{ .key }}
{{- end -}}

{{- define "gp.databaseAppEnv" -}}
{{- include "gp.secretEnv" (dict "name" "DATABASE_URL" "secret" (required "database.app.secretName is required (Secret with the glossa_app DSN)" .Values.database.app.secretName) "key" .Values.database.app.secretKey) }}
{{- end -}}

{{- define "gp.authEnv" -}}
{{- include "gp.secretEnv" (dict "name" "GLOSSA_AUTH_SECRET" "secret" (required "auth.secretName is required (Secret with GLOSSA_AUTH_SECRET)" .Values.auth.secretName) "key" .Values.auth.secretKey) }}
{{- end -}}

{{/* Object storage env; (dict "root" $ "creds" .Values.objectStorage.server). */}}
{{- define "gp.storageEnv" -}}
{{- $s := .root.Values.objectStorage -}}
- name: GLOSSA_STORAGE_DRIVER
  value: s3
- name: GLOSSA_S3_ENDPOINT
  value: {{ required "objectStorage.endpoint is required (host[:port], no scheme)" $s.endpoint | quote }}
- name: GLOSSA_S3_BUCKET
  value: {{ required "objectStorage.bucket is required" $s.bucket | quote }}
- name: GLOSSA_S3_REGION
  value: {{ $s.region | quote }}
{{- with $s.prefix }}
- name: GLOSSA_S3_PREFIX
  value: {{ . | quote }}
{{- end }}
- name: GLOSSA_S3_PATH_STYLE
  value: {{ $s.pathStyle | quote }}
- name: GLOSSA_S3_INSECURE
  value: {{ $s.insecure | quote }}
- name: GLOSSA_S3_TIMEOUT
  value: {{ $s.timeout | quote }}
{{ include "gp.secretEnv" (dict "name" "GLOSSA_S3_ACCESS_KEY_ID" "secret" .secretName "key" .creds.accessKeyIdKey) }}
{{ include "gp.secretEnv" (dict "name" "GLOSSA_S3_SECRET_ACCESS_KEY" "secret" .secretName "key" .creds.secretAccessKeyKey) }}
{{- end -}}

{{/* ── NetworkPolicy fragments ──────────────────────────────────── */}}

{{- define "gp.np.dnsEgress" -}}
- to:
    - namespaceSelector:
        {{- toYaml .Values.networkPolicy.dns.namespaceSelector | nindent 8 }}
      podSelector:
        {{- toYaml .Values.networkPolicy.dns.podSelector | nindent 8 }}
  ports:
    - port: 53
      protocol: UDP
    - port: 53
      protocol: TCP
{{- end -}}

{{/* An egress rule from {to, ports}; an empty `to` means any destination. */}}
{{- define "gp.np.egressRule" -}}
- ports:
    {{- toYaml .ports | nindent 4 }}
  {{- with .to }}
  to:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end -}}

{{/* Ingress from the ingress controller to a port. */}}
{{- define "gp.np.fromIngressController" -}}
- from:
    - namespaceSelector:
        {{- toYaml .root.Values.networkPolicy.ingressController.namespaceSelector | nindent 8 }}
      podSelector:
        {{- toYaml .root.Values.networkPolicy.ingressController.podSelector | nindent 8 }}
  ports:
    - port: {{ .port }}
      protocol: TCP
{{- end -}}

{{/* Ingress from metrics scrapers to a port. */}}
{{- define "gp.np.fromScrapers" -}}
- from:
    {{- toYaml .root.Values.metrics.allowFrom | nindent 4 }}
  ports:
    - port: {{ .port }}
      protocol: TCP
{{- end -}}
