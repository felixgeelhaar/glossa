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

{{/* DATABASE_URL (glossa_app): database.app's Secret, else with
     postgres.enabled a DSN for the in-namespace Postgres built from
     glossa_app's password. */}}
{{- define "gp.databaseAppEnv" -}}
{{- if .Values.database.app.secretName -}}
{{- include "gp.secretEnv" (dict "name" "DATABASE_URL" "secret" .Values.database.app.secretName "key" .Values.database.app.secretKey) }}
{{- else if .Values.postgres.enabled -}}
{{- include "gp.secretEnv" (dict "name" "GLOSSA_DB_APP_PASSWORD" "secret" (include "gp.postgres.appSecret" .) "key" .Values.postgres.app.passwordKey) }}
- name: DATABASE_URL
  value: {{ include "gp.postgres.dsn" (dict "root" . "user" "glossa_app" "passwordVar" "GLOSSA_DB_APP_PASSWORD" "extra" (printf "pool_max_conns=%d" (int .Values.postgres.app.poolMaxConns))) | quote }}
{{- else -}}
{{- fail "database.app.secretName is required (Secret with the glossa_app DSN), or set postgres.enabled" -}}
{{- end -}}
{{- end -}}

{{/* MIGRATION_DATABASE_URL (the schema owner), like gp.databaseAppEnv. */}}
{{- define "gp.databaseMigrationEnv" -}}
{{- if .Values.database.migration.secretName -}}
{{- include "gp.secretEnv" (dict "name" "MIGRATION_DATABASE_URL" "secret" .Values.database.migration.secretName "key" .Values.database.migration.secretKey) }}
{{- else if .Values.postgres.enabled -}}
{{- include "gp.secretEnv" (dict "name" "GLOSSA_DB_OWNER_PASSWORD" "secret" (include "gp.postgres.ownerSecret" .) "key" .Values.postgres.owner.passwordKey) }}
- name: MIGRATION_DATABASE_URL
  value: {{ include "gp.postgres.dsn" (dict "root" . "user" .Values.postgres.owner.username "passwordVar" "GLOSSA_DB_OWNER_PASSWORD" "extra" "") | quote }}
{{- else -}}
{{- fail "database.migration.secretName is required when server.migrations.enabled (Secret with the owner DSN), or set postgres.enabled" -}}
{{- end -}}
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

{{/* ── In-namespace Postgres ─────────────────────────────────────── */}}

{{/* Host of the in-namespace Postgres Service. */}}
{{- define "gp.postgres.host" -}}
{{- include "gp.componentName" (dict "root" . "component" "postgres") -}}
{{- end -}}

{{- define "gp.postgres.superuserSecret" -}}
{{- required "postgres.superuser.secretName is required with postgres.enabled (existing Secret with the superuser's password)" .Values.postgres.superuser.secretName -}}
{{- end -}}

{{- define "gp.postgres.ownerSecret" -}}
{{- required "postgres.owner.secretName is required with postgres.enabled (existing Secret with the schema owner's password)" .Values.postgres.owner.secretName -}}
{{- end -}}

{{- define "gp.postgres.appSecret" -}}
{{- required "postgres.app.secretName is required with postgres.enabled (existing Secret with glossa_app's password)" .Values.postgres.app.secretName -}}
{{- end -}}

{{/* A keyword/value DSN for the in-namespace Postgres; the password is a
     $(VAR) reference to an env var defined before it (Kubernetes expands
     it). Keyword/value rather than a URL: a password needs no percent-
     encoding, only no ' or \ (the bootstrap Job refuses those).
     (dict "root" $ "user" "…" "passwordVar" "…" "extra" "k=v …") */}}
{{- define "gp.postgres.dsn" -}}
{{- $r := .root -}}
{{- $dsn := printf "host=%s port=5432 dbname=%s user=%s password='$(%s)' sslmode=disable" (include "gp.postgres.host" $r) $r.Values.postgres.database .user .passwordVar -}}
{{- if .extra }}{{ $dsn = printf "%s %s" $dsn .extra }}{{ end -}}
{{- $dsn -}}
{{- end -}}

{{/* libpq env (PGHOST, PGPORT, PGDATABASE, PGUSER, PGPASSWORD) for psql
     and pg_dump against the in-namespace Postgres:
     (dict "root" $ "user" "…" "secret" "…" "key" "…" "database" "…"). */}}
{{- define "gp.postgres.libpqEnv" -}}
- name: PGHOST
  value: {{ include "gp.postgres.host" .root | quote }}
- name: PGPORT
  value: "5432"
- name: PGDATABASE
  value: {{ .database | default .root.Values.postgres.database | quote }}
- name: PGUSER
  value: {{ .user | quote }}
{{ include "gp.secretEnv" (dict "name" "PGPASSWORD" "secret" .secret "key" .key) }}
- name: PGCONNECT_TIMEOUT
  value: "10"
{{- end -}}

{{/* When the database hooks (Postgres bootstrap, migrations, glossa_app's
     login) run. An external database exists before the release, so they
     run pre-install. The in-namespace Postgres is created BY the release,
     so on the first install they can only run post-install; on upgrades
     it already exists and they run pre-upgrade, before the new server
     version rolls out. */}}
{{- define "gp.db.hook" -}}
{{- ternary "post-install,pre-upgrade" "pre-install,pre-upgrade" (not (not .Values.postgres.enabled)) -}}
{{- end -}}

{{/* The pod securityContext of the postgres image (alpine: uid/gid 70). */}}
{{- define "gp.postgres.uid" -}}70{{- end -}}

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

{{/* Egress to Postgres: the in-namespace Postgres pod on 5432 with
     postgres.enabled (replacing networkPolicy.egress.postgres), else
     networkPolicy.egress.postgres. */}}
{{- define "gp.np.postgresEgress" -}}
{{- if .Values.postgres.enabled -}}
- to:
    - podSelector:
        matchLabels:
          {{- include "gp.selectorLabels" (dict "root" . "component" "postgres") | nindent 10 }}
  ports:
    - port: 5432
      protocol: TCP
{{- else -}}
{{- include "gp.np.egressRule" .Values.networkPolicy.egress.postgres -}}
{{- end -}}
{{- end -}}

{{/* A NetworkPolicy for a hook pod: no ingress; egress to DNS plus the
     given rules. (dict "root" $ "component" "…" "hook" "…" "egress" "<yaml>") */}}
{{- define "gp.np.hookPolicy" -}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "gp.componentName" (dict "root" .root "component" .component) }}
  labels:
    {{- include "gp.componentLabels" (dict "root" .root "component" .component) | nindent 4 }}
  annotations:
    helm.sh/hook: {{ .hook }}
    helm.sh/hook-weight: "-10"
    helm.sh/hook-delete-policy: before-hook-creation
spec:
  podSelector:
    matchLabels:
      {{- include "gp.selectorLabels" (dict "root" .root "component" .component) | nindent 6 }}
  policyTypes: [Ingress, Egress]
  ingress: []
  egress:
    {{- include "gp.np.dnsEgress" .root | nindent 4 }}
    {{- .egress | nindent 4 }}
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
