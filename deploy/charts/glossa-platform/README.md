# glossa-platform

Helm chart for Glossa's platform (RFC 0002 §3, §11): **glossa-server**
(control plane and the `/v1` API), **glossa-edge** (the stateless
delivery plane) and **Studio** (the web app). It bundles no Postgres and
no object storage; both are external and referenced through Secrets.

The v0.3 chart (`deploy/charts/glossa`) is separate and unchanged.

```text
                  ┌─────────────── traefik (websecure) ────────────────┐
app.<domain>/v1/* │→ glossa-server :8080 ──→ Postgres (glossa_app, RLS) │
app.<domain>/*    │→ studio        :8080     object storage (read-write)│
api.<domain>/v1/* │→ glossa-server :8080     SMTP (optional)            │
cdn.<domain>/v1/* │→ glossa-edge   :8081 ──→ object storage (read-only) │
                  └─────────────────────────────────────────────────────┘
pre-install/pre-upgrade hook: glossa-server -migrate=only (schema owner)
```

What it renders:

| Resource | server | edge | studio | notes |
|---|---|---|---|---|
| Deployment + Service (`http`) | ✓ | ✓ | ✓ | `<fullname>-server`, `-edge`, `-studio` |
| PodDisruptionBudget | ✓ | ✓ | ✓ | `maxUnavailable: 1` |
| NetworkPolicy (ingress + egress) | ✓ | ✓ | ✓ | see [Network policies](#network-policies) |
| IngressRoute (traefik) | api host | cdn host | studio host (+ `/v1` → server) | plus an http→https redirect route |
| Certificate (cert-manager) | ✓ | ✓ | ✓ | one per host |
| Job (hook) + its NetworkPolicy | migrations | | | `<fullname>-migrate` |
| HorizontalPodAutoscaler | | optional | | `edge.autoscaling.enabled` |
| ServiceMonitor | optional | optional | | `metrics.serviceMonitor.enabled` |

Every pod meets PodSecurity **restricted**: non-root (65532 for the Go
images, 101 for Studio's nginx), `seccompProfile: RuntimeDefault`, all
capabilities dropped, no privilege escalation, **read-only root
filesystem** (an `emptyDir` at `/tmp`), no service account token, no
service-link env vars. Probes: `GET /livez` (startup, liveness) and
`GET /readyz` (readiness) on the server and edge, `GET /healthz` on
Studio. The server drains on SIGTERM within `server.shutdownTimeout`,
below `terminationGracePeriodSeconds`.

## Images

Built by `.github/workflows/release.yml` (push to `main`, `v*.*.*` tags,
manual) and pushed to GHCR; pull requests only build them
(`.github/workflows/platform.yml`).

| Image | Dockerfile | Base (pinned by digest) | User | Port |
|---|---|---|---|---|
| `ghcr.io/felixgeelhaar/glossa-server` | `platform/Dockerfile.server` | `gcr.io/distroless/static-debian12:nonroot` | 65532 | 8080 |
| `ghcr.io/felixgeelhaar/glossa-edge` | `platform/Dockerfile.edge` | `gcr.io/distroless/static-debian12:nonroot` | 65532 | 8081 |
| `ghcr.io/felixgeelhaar/glossa-studio` | `studio/Dockerfile` | `nginxinc/nginx-unprivileged:1.29-alpine` | 101 | 8080 |

All three build from the repository root, e.g.
`docker build -f platform/Dockerfile.server .`. Tags: `<semver>` and
`<major>.<minor>` on release tags (plus `latest`), `main-<sha>` on main.

**Studio at runtime.** The image renders `GLOSSA_STUDIO_*` variables at
start, so one image serves every environment: `/config.json`
(`apiBaseUrl`, `edgeUrl`, `environment`) and the CSP's `connect-src`
(`GLOSSA_STUDIO_CSP_CONNECT_SRC`). The CSP is strict: `default-src
'none'`, scripts, styles and fonts from `'self'` only, no inline code, no
eval, `frame-ancestors 'none'`. Studio's `/v1` is expected on its own
origin; the chart routes it to glossa-server. nginx answers `/v1` itself
with a 502 problem if that routing is missing. With `docker run
--read-only`, mount a tmpfs at `/tmp` (startup fails loudly otherwise).

## First install

Order matters: the migration Job runs before any other resource of the
release exists and needs its Secrets and database already in place.

1. **Cluster prerequisites.** traefik (k3s ships it), cert-manager with a
   `ClusterIssuer` named `letsencrypt-prod` (or set
   `ingress.certManager.*`). Optional: Prometheus Operator for
   `ServiceMonitor`s.
2. **Namespace**, enforcing the restricted profile:

   ```sh
   kubectl create namespace glossa-platform
   kubectl label namespace glossa-platform \
     pod-security.kubernetes.io/enforce=restricted \
     pod-security.kubernetes.io/audit=restricted \
     pod-security.kubernetes.io/warn=restricted
   ```

3. **DNS.** Point `hosts.studio`, `hosts.api` and `hosts.cdn` at the
   ingress's public address. cert-manager's HTTP-01 challenge needs them
   to resolve before the Certificates can be issued.
4. **Database and roles** (Postgres 16). The schema owner runs migrations
   and must have `CREATEROLE` but must **not** be a superuser; the app role
   must be `NOSUPERUSER NOBYPASSRLS` (the server refuses to start
   otherwise). The first migration creates `glossa_app` and
   `glossa_system` as `NOLOGIN` only if they are missing, so create
   `glossa_app` with its login up front and the first install comes up in
   one go:

   ```sql
   CREATE ROLE glossa_owner LOGIN CREATEROLE PASSWORD '…';
   CREATE DATABASE glossa OWNER glossa_owner;
   CREATE ROLE glossa_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD '…';
   ```

   With CloudNativePG: make `glossa_owner` the `bootstrap.initdb` owner and
   also list it under `managed.roles` with `createrole: true`; declare
   `glossa_app` there too, with `login: true`, `superuser: false`,
   `bypassrls: false` and a `passwordSecret`.
5. **Bucket and credentials.** Create the bucket (and prefix, if shared).
   Create two credentials: read-write for glossa-server, **read-only** for
   glossa-edge.
6. **Secrets** (names are the defaults of the example values; any name
   works, see [Values](#values)):

   ```sh
   NS=glossa-platform
   kubectl -n $NS create secret generic glossa-db-app \
     --from-literal=DATABASE_URL='postgres://glossa_app:…@<host>:5432/glossa?sslmode=require&pool_max_conns=20'
   kubectl -n $NS create secret generic glossa-db-owner \
     --from-literal=MIGRATION_DATABASE_URL='postgres://glossa_owner:…@<host>:5432/glossa?sslmode=require'
   # Keep this value: it seals enrolled TOTP secrets.
   kubectl -n $NS create secret generic glossa-auth \
     --from-literal=GLOSSA_AUTH_SECRET="$(openssl rand -base64 32)"
   # keyId=base64(32-byte Ed25519 seed); runtimes pin the public key.
   kubectl -n $NS create secret generic glossa-release-signing \
     --from-literal=GLOSSA_RELEASE_SIGNING_KEYS="k1=$(openssl rand -base64 32)"
   # Only with an SMTP server (see Mail):
   kubectl -n $NS create secret generic glossa-smtp \
     --from-literal=GLOSSA_SMTP_USERNAME='…' --from-literal=GLOSSA_SMTP_PASSWORD='…'
   kubectl -n $NS create secret generic glossa-s3-rw \
     --from-literal=GLOSSA_S3_ACCESS_KEY_ID='…' --from-literal=GLOSSA_S3_SECRET_ACCESS_KEY='…'
   kubectl -n $NS create secret generic glossa-s3-ro \
     --from-literal=GLOSSA_S3_ACCESS_KEY_ID='…' --from-literal=GLOSSA_S3_SECRET_ACCESS_KEY='…'
   ```

   Back up `GLOSSA_AUTH_SECRET` and the signing seeds outside the cluster.
   Percent-encode special characters in DSN passwords.
7. **Install:**

   ```sh
   helm upgrade --install glossa-platform deploy/charts/glossa-platform \
     --namespace glossa-platform -f my-values.yaml --wait --timeout 10m
   ```

   The hook Job migrates first; server pods start after it succeeds. A
   failed Job is kept for `kubectl logs job/<fullname>-migrate`.
8. **Verify:** `https://<studio>` loads and signs in (magic link),
   `https://<api>/v1/…` answers, `https://<cdn>/v1/<key>/production/manifest.json`
   answers 404 for an unknown key. `/metrics`, `/livez` and `/readyz` are
   not routed publicly.

Upgrades run the same Job before the new server version starts; there is
no down-migration step in the chart. Rotating signing keys: add the new
key to the Secret (both sign), move runtimes to it, then move the old one
to `release.retiredKeys` (platform/README.md, "Release").

Uninstall leaves the migration NetworkPolicy (a hook resource) and never
touches the database, the bucket or the Secrets.

## Deploying with RollOps

Klarlabs rolls Glossa out with RollOps, as it does v0.3 (`.rollops/*.yaml`
at the repository root); the chart carries no Keel or other rollout-tool
annotations. RollOps RolloutConfigs are **generated from `helm template`
output**, one per rendered resource, each embedding the manifest as
`spec.target.spec.manifest` (and `spec.target.spec.image` for workloads),
with every image **pinned by digest** once it exists in GHCR:

```sh
helm template glossa-platform deploy/charts/glossa-platform \
  --namespace glossa-platform -f my-values.yaml --skip-tests \
  --set server.image.digest=sha256:… \
  --set edge.image.digest=sha256:… \
  --set studio.image.digest=sha256:…
```

The chart's Helm hooks are plain resources in that output and do not run
by themselves: the migration Job (`<fullname>-migrate`, with its
NetworkPolicy) must complete before the server Deployment rolls, and a
Job is immutable, so it is deleted and re-created per rollout.

## Mail

SMTP is optional. With `mail.smtp.addr` set (and optionally
`mail.smtp.secretName` for AUTH), the chart sets `GLOSSA_MAIL_DRIVER=smtp`,
the `GLOSSA_SMTP_*` variables and the server's SMTP egress rule. Without
it, the chart sets **no** `GLOSSA_MAIL_*`/`GLOSSA_SMTP_*` variable, needs
no SMTP Secret and opens no SMTP egress: glossa-server's own behaviour
without a mailer applies. Email sign-in links and invitations are not
delivered then. glossa-server 0.4.0 still defaults to the `log` driver,
which writes sign-in links to its log; NOTES.txt says so after install.
`mail.driver: log` can be set explicitly for development.

## Network policies

With `networkPolicy.enabled`, each component gets one policy covering both
directions; anything not listed is denied.

| Pod | Ingress | Egress |
|---|---|---|
| server | ingress controller → 8080; `metrics.allowFrom` → 8080 | DNS; `egress.postgres`; `egress.objectStorage`; `egress.smtp` (SMTP configured); `egress.otlp` (endpoint set) |
| edge | ingress controller → 8081; `metrics.allowFrom` → 8081 | DNS; `egress.objectStorage`; `egress.otlp` (endpoint set) |
| studio | ingress controller → 8080 | none |
| migrate Job | none | DNS; `egress.postgres` |

NetworkPolicy matches IPs, not hostnames. Each `egress.*.to` takes
NetworkPolicyPeers (`ipBlock` for external services, namespace/pod
selectors for in-cluster ones such as CNPG or MinIO). **An empty `to`
allows those ports to any destination**; NOTES.txt warns while Postgres
or object storage is unpinned. Most policy engines let the kubelet's
probes through regardless; if yours doesn't, allow the node addresses
with `networkPolicy.extraIngress`.

## Values

REQUIRED values have no default; rendering fails with a message naming
the value until it is set.

### Hosts and images

| Value | Default | Meaning |
|---|---|---|
| `hosts.studio` | REQUIRED | Studio host (`app.<domain>`). Also `GLOSSA_STUDIO_URL`, default passkey RP ID and origin. |
| `hosts.api` | REQUIRED | Public API host (`api.<domain>`), `/v1` only. |
| `hosts.cdn` | REQUIRED | glossa-edge host (`cdn.<domain>`), `/v1` only. |
| `nameOverride` | `""` | `app.kubernetes.io/name` (default: chart name). |
| `fullnameOverride` | `""` | Prefix of every resource name (default: release name). |
| `image.registry` | `ghcr.io` | Registry of all three images. |
| `image.pullPolicy` | `IfNotPresent` | |
| `image.pullSecrets` | `[]` | `imagePullSecrets` for every pod. |
| `<component>.image.repository` | `felixgeelhaar/glossa-{server,edge,studio}` | `<component>` is `server`, `edge` or `studio`. |
| `<component>.image.tag` | `""` → `appVersion` | |
| `<component>.image.digest` | `""` | `sha256:…`; appended as `@digest`. |
| `commonLabels` | `{}` | Labels on every resource. |
| `deploymentAnnotations` | `{}` | Annotations on every Deployment. |
| `podAnnotations` | `{}` | Annotations on every pod. |

### Per component (`server`, `edge`, `studio`)

| Value | Default | Meaning |
|---|---|---|
| `<component>.replicas` | `2` | Ignored for the edge when autoscaling is on. |
| `<component>.resources` | see values.yaml | Requests and memory limits; no CPU limits. |
| `<component>.extraEnv` | `[]` | Extra `EnvVar`s (any variable from platform/README.md). |
| `<component>.pdb.enabled` / `.maxUnavailable` | `true` / `1` | PodDisruptionBudget. |
| `<component>.nodeSelector` / `.tolerations` / `.affinity` | empty | Scheduling. |
| `<component>.topologySpreadConstraints` | `[]` | Replaces the default soft spread across nodes. |

### glossa-server

| Value | Default | Env / meaning |
|---|---|---|
| `server.logLevel` | `info` | `GLOSSA_LOG_LEVEL` |
| `server.shutdownTimeout` | `25s` | `GLOSSA_SHUTDOWN_TIMEOUT` |
| `server.terminationGracePeriodSeconds` | `35` | Must exceed the shutdown timeout. |
| `server.sessionTTL` | `336h` | `GLOSSA_SESSION_TTL` |
| `server.outbox.enabled` | `true` | `GLOSSA_OUTBOX_ENABLED` (dispatchers claim with leases, so replicas share work). |
| `server.webauthn.rpId` | `hosts.studio` | `GLOSSA_WEBAUTHN_RP_ID`; changing it later invalidates enrolled passkeys. |
| `server.webauthn.rpName` | `Glossa` | `GLOSSA_WEBAUTHN_RP_NAME` |
| `server.webauthn.origins` | `[https://<hosts.studio>]` | `GLOSSA_WEBAUTHN_ORIGINS` |
| `server.otel.endpoint` | `""` | `OTEL_EXPORTER_OTLP_ENDPOINT`; also opens `egress.otlp`. |
| `server.migrations.enabled` | `true` | Hook Job `glossa-server -migrate=only`. |
| `server.migrations.backoffLimit` | `3` | Retries (a fresh pod's NetworkPolicy may lag). |
| `server.migrations.activeDeadlineSeconds` | `600` | |
| `server.migrations.resources` | see values.yaml | |

### glossa-edge

| Value | Default | Env / meaning |
|---|---|---|
| `edge.logLevel` / `edge.shutdownTimeout` / `edge.terminationGracePeriodSeconds` | `info` / `25s` / `35` | As for the server. |
| `edge.cache.bytes` | `67108864` | `GLOSSA_EDGE_CACHE_BYTES`; keep the memory limit above it. |
| `edge.cache.keyTTL` | `30s` | `GLOSSA_EDGE_KEY_TTL`: how long a revoked key keeps working. |
| `edge.cache.manifestTTL` | `5s` | `GLOSSA_EDGE_MANIFEST_TTL`: publish delay per edge. |
| `edge.otel.endpoint` | `""` | `OTEL_EXPORTER_OTLP_ENDPOINT` |
| `edge.autoscaling.enabled` | `false` | HPA on CPU. |
| `edge.autoscaling.minReplicas` / `.maxReplicas` | `2` / `6` | |
| `edge.autoscaling.targetCPUUtilizationPercentage` | `70` | |

### Studio

| Value | Default | Env / meaning |
|---|---|---|
| `studio.runtimeConfig.apiBaseUrl` | `""` | `GLOSSA_STUDIO_API_BASE_URL` → `/config.json`; empty means `/v1` on Studio's origin. |
| `studio.runtimeConfig.edgeUrl` | `https://<hosts.cdn>` | `GLOSSA_STUDIO_EDGE_URL` → `/config.json`. |
| `studio.runtimeConfig.environment` | `production` | `GLOSSA_STUDIO_ENVIRONMENT` → `/config.json`. |
| `studio.runtimeConfig.cspConnectSrc` | `""` | `GLOSSA_STUDIO_CSP_CONNECT_SRC`: extra `connect-src` origins, space separated. |

### Secrets and external services

| Value | Default | Meaning |
|---|---|---|
| `database.app.secretName` / `.secretKey` | REQUIRED / `DATABASE_URL` | glossa_app DSN (server). Pool size goes in the DSN (`pool_max_conns`). |
| `database.migration.secretName` / `.secretKey` | REQUIRED with migrations / `MIGRATION_DATABASE_URL` | Owner DSN; mounted into the migration Job only. |
| `auth.secretName` / `.secretKey` | REQUIRED / `GLOSSA_AUTH_SECRET` | Base64 of ≥ 32 random bytes. |
| `release.signingKeys.secretName` / `.secretKey` | REQUIRED / `GLOSSA_RELEASE_SIGNING_KEYS` | `keyId=base64(seed)`, comma-separated. |
| `release.retiredKeys` | `""` | `GLOSSA_RELEASE_RETIRED_KEYS` (public keys, not secret). |
| `mail.driver` | `""` → `smtp` if `mail.smtp.addr` is set, else unset | `GLOSSA_MAIL_DRIVER`; `log` is for development only. See [Mail](#mail). |
| `mail.from` | `""` | `GLOSSA_MAIL_FROM` (set only with a driver). |
| `mail.smtp.addr` | `""` | `GLOSSA_SMTP_ADDR` (`host:587`, STARTTLS). Setting it turns SMTP on; required with `mail.driver: smtp`. |
| `mail.smtp.secretName` / `.usernameKey` / `.passwordKey` | `""` / `GLOSSA_SMTP_USERNAME` / `GLOSSA_SMTP_PASSWORD` | AUTH credentials; empty name: no AUTH. |
| `objectStorage.endpoint` | REQUIRED | `GLOSSA_S3_ENDPOINT`, `host[:port]` without scheme. |
| `objectStorage.bucket` | REQUIRED | `GLOSSA_S3_BUCKET` |
| `objectStorage.region` | `us-east-1` | `GLOSSA_S3_REGION` |
| `objectStorage.prefix` | `""` | `GLOSSA_S3_PREFIX` |
| `objectStorage.pathStyle` | `false` | `GLOSSA_S3_PATH_STYLE` (MinIO: `true`). |
| `objectStorage.insecure` | `false` | `GLOSSA_S3_INSECURE`; local clusters only. |
| `objectStorage.timeout` | `10s` | `GLOSSA_S3_TIMEOUT` |
| `objectStorage.server.secretName` / `.accessKeyIdKey` / `.secretAccessKeyKey` | REQUIRED / `GLOSSA_S3_ACCESS_KEY_ID` / `GLOSSA_S3_SECRET_ACCESS_KEY` | Read-write credentials. |
| `objectStorage.edge.secretName` / keys | `""` → server's / same | Read-only credentials for the edge (recommended). |

### Ingress

| Value | Default | Meaning |
|---|---|---|
| `ingress.enabled` | `true` | IngressRoutes, Middlewares, Certificates. |
| `ingress.entryPoints.web` / `.websecure` | `web` / `websecure` | traefik entry points. |
| `ingress.httpsRedirect` | `true` | Permanent http→https (ACME HTTP-01 paths excepted). |
| `ingress.compress.studio` / `.api` / `.cdn` | `true` / `true` / `false` | traefik compression per host (the edge's strong ETags describe uncompressed bytes). |
| `ingress.hsts.enabled` / `.maxAge` / `.includeSubdomains` / `.preload` | `true` / `31536000` / `false` / `false` | HSTS on all three hosts. |
| `ingress.certManager.enabled` | `true` | One Certificate per host. |
| `ingress.certManager.issuerName` / `.issuerKind` | `letsencrypt-prod` / `ClusterIssuer` | |
| `ingress.tlsSecretNames.studio` / `.api` / `.cdn` | `<fullname>-<host>-tls` | TLS Secret per host. |
| `ingress.extraMiddlewares.studio` / `.api` / `.cdn` | `[]` | Extra middleware refs (`{name, namespace}`), e.g. rate limiting on the API. |

### Network policies and metrics

| Value | Default | Meaning |
|---|---|---|
| `networkPolicy.enabled` | `true` | Policies for all components and the migration Job. |
| `networkPolicy.ingressController.namespaceSelector` / `.podSelector` | kube-system / `app.kubernetes.io/name: traefik` | Where traffic may come from. |
| `networkPolicy.dns.namespaceSelector` / `.podSelector` | kube-system / `k8s-app: kube-dns` | Cluster DNS (53/UDP+TCP). |
| `networkPolicy.egress.postgres` | `to: []`, 5432 | Server and migration Job. |
| `networkPolicy.egress.objectStorage` | `to: []`, 443 | Server and edge. |
| `networkPolicy.egress.smtp` | `to: []`, 587 | Server, only when SMTP is configured. |
| `networkPolicy.egress.otlp` | `to: []`, 4318 | Server/edge, when their OTel endpoint is set. |
| `networkPolicy.extraIngress.{server,edge,studio}` | `[]` | Extra `NetworkPolicyIngressRule`s. |
| `networkPolicy.extraEgress.{server,edge}` | `[]` | Extra `NetworkPolicyEgressRule`s. |
| `metrics.allowFrom` | `[]` | Peers that may scrape `/metrics` on server and edge. |
| `metrics.serviceMonitor.enabled` | `false` | ServiceMonitors for server and edge. |
| `metrics.serviceMonitor.interval` / `.scrapeTimeout` / `.labels` | `30s` / `10s` / `{}` | |

## Testing the chart

```sh
helm lint deploy/charts/glossa-platform --strict -f deploy/charts/glossa-platform/ci/minimal-values.yaml
helm template t deploy/charts/glossa-platform -f deploy/charts/glossa-platform/ci/full-values.yaml \
  | kubeconform -strict -summary -schema-location default \
      -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
```

`ci/*-values.yaml` follow chart-testing's layout (`ct lint` picks them
up); `examples/values-klarlabs.yaml` is the Klarlabs starting point, with
`TODO(infra)` markers for the decisions still open. CI runs lint,
template and kubeconform for all three files on every pull request that
touches the chart.
