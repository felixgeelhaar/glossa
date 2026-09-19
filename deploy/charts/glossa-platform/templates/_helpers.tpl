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

{{/* An image outside image.registry, by full repository:
     (dict "image" .Values.minio.image). */}}
{{- define "gp.externalImage" -}}
{{- $ref := printf "%s:%s" .image.repository .image.tag -}}
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

{{/* ── Object storage ───────────────────────────────────────────── */}}

{{/* host:port of the in-namespace MinIO Service. */}}
{{- define "gp.minio.service" -}}
{{- printf "%s:9000" (include "gp.componentName" (dict "root" . "component" "minio")) -}}
{{- end -}}

{{/* GLOSSA_S3_ENDPOINT: objectStorage.endpoint, which defaults to the
     in-namespace MinIO when minio.enabled. */}}
{{- define "gp.storage.endpoint" -}}
{{- if .Values.minio.enabled -}}
{{- .Values.objectStorage.endpoint | default (include "gp.minio.service" .) -}}
{{- else -}}
{{- required "objectStorage.endpoint is required (host[:port], no scheme), or set minio.enabled" .Values.objectStorage.endpoint -}}
{{- end -}}
{{- end -}}

{{/* MinIO's S3 URL as mc sees it: in-cluster plain HTTP. */}}
{{- define "gp.minio.url" -}}
{{- printf "http://%s" (include "gp.storage.endpoint" .) -}}
{{- end -}}

{{- define "gp.storage.bucket" -}}
{{- required "objectStorage.bucket is required" .Values.objectStorage.bucket -}}
{{- end -}}

{{- define "gp.storage.serverSecret" -}}
{{- required "objectStorage.server.secretName is required (Secret with read-write S3 credentials)" .Values.objectStorage.server.secretName -}}
{{- end -}}

{{/* glossa-edge's credentials as YAML {secretName, accessKeyIdKey,
     secretAccessKeyKey}: objectStorage.edge, else the server's. With
     MinIO the chart provisions a separate read-only user, so the edge
     must have its own Secret then. */}}
{{- define "gp.storage.edgeCreds" -}}
{{- $s := .Values.objectStorage -}}
{{- if $s.edge.secretName -}}
{{- if and .Values.minio.enabled (eq $s.edge.secretName $s.server.secretName) (eq $s.edge.accessKeyIdKey $s.server.accessKeyIdKey) -}}
{{- fail "objectStorage.edge reuses objectStorage.server's access key (same Secret and key); with minio.enabled the edge gets its own read-only user" -}}
{{- end -}}
{{- toYaml $s.edge -}}
{{- else if .Values.minio.enabled -}}
{{- fail "objectStorage.edge.secretName is required with minio.enabled (glossa-edge's read-only MinIO user)" -}}
{{- else -}}
{{- $_ := required "objectStorage.edge.secretName or objectStorage.server.secretName is required" $s.server.secretName -}}
{{- toYaml $s.server -}}
{{- end -}}
{{- end -}}

{{/* Env of the mc containers (bootstrap Job, helm test): MinIO, the
     bucket and both application users' credentials. */}}
{{- define "gp.minio.mcEnv" -}}
{{- $s := .Values.objectStorage -}}
{{- $edge := include "gp.storage.edgeCreds" . | fromYaml -}}
{{- $serverSecret := include "gp.storage.serverSecret" . -}}
- name: MINIO_URL
  value: {{ include "gp.minio.url" . | quote }}
- name: BUCKET
  value: {{ include "gp.storage.bucket" . | quote }}
- name: OBJECT_PREFIX
  value: {{ trimAll "/" $s.prefix | quote }}
- name: MC_CONFIG_DIR
  value: /tmp/.mc
- name: HOME
  value: /tmp
{{ include "gp.secretEnv" (dict "name" "RW_ACCESS_KEY" "secret" $serverSecret "key" $s.server.accessKeyIdKey) }}
{{ include "gp.secretEnv" (dict "name" "RW_SECRET_KEY" "secret" $serverSecret "key" $s.server.secretAccessKeyKey) }}
{{ include "gp.secretEnv" (dict "name" "RO_ACCESS_KEY" "secret" $edge.secretName "key" $edge.accessKeyIdKey) }}
{{ include "gp.secretEnv" (dict "name" "RO_SECRET_KEY" "secret" $edge.secretName "key" $edge.secretAccessKeyKey) }}
{{- end -}}

{{/* Object storage env; (dict "root" $ "creds" .Values.objectStorage.server "secretName" "…"). */}}
{{- define "gp.storageEnv" -}}
{{- $s := .root.Values.objectStorage -}}
{{- $minio := .root.Values.minio.enabled -}}
- name: GLOSSA_STORAGE_DRIVER
  value: s3
- name: GLOSSA_S3_ENDPOINT
  value: {{ include "gp.storage.endpoint" .root | quote }}
- name: GLOSSA_S3_BUCKET
  value: {{ include "gp.storage.bucket" .root | quote }}
- name: GLOSSA_S3_REGION
  value: {{ $s.region | quote }}
{{- with $s.prefix }}
- name: GLOSSA_S3_PREFIX
  value: {{ . | quote }}
{{- end }}
{{- /* The in-namespace MinIO: path-style requests and plain HTTP on the
       pod network, where NetworkPolicy admits only server and edge. */}}
- name: GLOSSA_S3_PATH_STYLE
  value: {{ or $minio $s.pathStyle | quote }}
- name: GLOSSA_S3_INSECURE
  value: {{ or $minio $s.insecure | quote }}
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

{{/* Egress to object storage: the in-namespace MinIO pod on 9000 with
     minio.enabled (replacing networkPolicy.egress.objectStorage), else
     networkPolicy.egress.objectStorage. */}}
{{- define "gp.np.objectStorageEgress" -}}
{{- if .Values.minio.enabled -}}
- to:
    - podSelector:
        matchLabels:
          {{- include "gp.selectorLabels" (dict "root" . "component" "minio") | nindent 10 }}
  ports:
    - port: 9000
      protocol: TCP
{{- else -}}
{{- include "gp.np.egressRule" .Values.networkPolicy.egress.objectStorage -}}
{{- end -}}
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
