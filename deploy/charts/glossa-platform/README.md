# glossa-platform

Helm chart for Glossa's platform (RFC 0002 §3, §11): **glossa-server**
(control plane and the `/v1` API), **glossa-edge** (the stateless
delivery plane) and **Studio** (the web app). It bundles no Postgres:
the database is external and referenced through Secrets. Object storage
is either external S3 or, with `minio.enabled`, one MinIO in the
release's namespace (see [In-namespace MinIO](#in-namespace-minio)).

The v0.3 chart (`deploy/charts/glossa`) is separate and unchanged.

```text
                  ┌─────────────── traefik (websecure) ────────────────┐
app.<domain>/v1/* │→ glossa-server :8080 ──→ Postgres (glossa_app, RLS) │
app.<domain>/*    │→ studio        :8080     object storage (read-write)│
api.<domain>/v1/* │→ glossa-server :8080     SMTP (optional)            │
cdn.<domain>/v1/* │→ glossa-edge   :8081 ──→ object storage (read-only) │
                  └─────────────────────────────────────────────────────┘
pre-install/pre-upgrade hook:   glossa-server -migrate=only (schema owner)
post-install/post-upgrade hook: MinIO bootstrap (bucket, users; minio.enabled)
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

With `minio.enabled` it adds `<fullname>-minio`: a StatefulSet with a
Longhorn volume, a ClusterIP and a headless Service (S3, 9000), a
PodDisruptionBudget and a NetworkPolicy; the bootstrap Job
`<fullname>-minio-bootstrap` (hook) with its NetworkPolicy; and the
`helm test` Pod `<fullname>-minio-test` with its NetworkPolicy.

Every pod meets PodSecurity **restricted**: non-root (65532 for the Go
images, 101 for Studio's nginx, 1000 for MinIO and mc), `seccompProfile: RuntimeDefault`, all
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

   With CloudNativePG, see [Postgres with CloudNativePG](#postgres-with-cloudnativepg).
5. **Bucket and credentials.** External storage: create the bucket (and
   prefix, if shared) and two credentials, read-write for glossa-server
   and **read-only** for glossa-edge. With `minio.enabled`, create only
   the three MinIO Secrets below; the bootstrap Job creates the bucket and
   a MinIO user for each of the two credential Secrets after the install.
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

   With `minio.enabled` the chart makes glossa-s3-rw and glossa-s3-ro MinIO
   users, so generate them, and add MinIO's root credentials (never used
   by glossa itself). MinIO wants access keys of at least 3 and secret keys
   of 8–40 characters; the three access keys must differ:

   ```sh
   kubectl -n $NS create secret generic glossa-minio-root \
     --from-literal=MINIO_ROOT_USER=glossa-root \
     --from-literal=MINIO_ROOT_PASSWORD="$(openssl rand -hex 20)"
   kubectl -n $NS create secret generic glossa-s3-rw \
     --from-literal=GLOSSA_S3_ACCESS_KEY_ID=glossa-server \
     --from-literal=GLOSSA_S3_SECRET_ACCESS_KEY="$(openssl rand -hex 20)"
   kubectl -n $NS create secret generic glossa-s3-ro \
     --from-literal=GLOSSA_S3_ACCESS_KEY_ID=glossa-edge \
     --from-literal=GLOSSA_S3_SECRET_ACCESS_KEY="$(openssl rand -hex 20)"
   ```

   Back up `GLOSSA_AUTH_SECRET` and the signing seeds outside the cluster.
   Percent-encode special characters in DSN passwords.
7. **Install:**

   ```sh
   helm upgrade --install glossa-platform deploy/charts/glossa-platform \
     --namespace glossa-platform -f my-values.yaml --wait --timeout 10m
   ```

   The hook Job migrates first; server pods start after it succeeds. A
   failed Job is kept for `kubectl logs job/<fullname>-migrate`. With
   `minio.enabled`, `--wait` waits for MinIO too, then the bootstrap Job
   creates the bucket and users (neither server nor edge needs them to
   become ready); `kubectl logs job/<fullname>-minio-bootstrap` if it
   fails. The whole install, Longhorn volume attach included, has to fit
   in `--timeout`.
8. **Verify:** with `minio.enabled`, `helm test glossa-platform -n
   glossa-platform` checks that the edge's credentials read the bucket but
   cannot write or delete. `https://<studio>` loads and signs in,
   `https://<api>/v1/…` answers, `https://<cdn>/v1/<key>/production/manifest.json`
   answers 404 for an unknown key. `/metrics`, `/livez` and `/readyz` are
   not routed publicly.

Upgrades run the same Job before the new server version starts; there is
no down-migration step in the chart. Rotating signing keys: add the new
key to the Secret (both sign), move runtimes to it, then move the old one
to `release.retiredKeys` (platform/README.md, "Release").

Uninstall leaves the hook NetworkPolicies (migration, MinIO bootstrap and
test) and never touches the database, the bucket or the Secrets. MinIO's
PersistentVolumeClaim (`data-<fullname>-minio-0`) is kept too; deleting it
deletes the data (Longhorn's reclaim policy is `Delete`).

### Postgres with CloudNativePG

The chart does not create the database. With the
[CloudNativePG](https://cloudnative-pg.io) operator installed, a minimal
cluster in the release's namespace (Klarlabs: `glossa-pg` in
`glossa-platform`):

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: glossa-pg
  namespace: glossa-platform
spec:
  instances: 1
  imageName: ghcr.io/cloudnative-pg/postgresql:16   # pin by digest
  storage:
    storageClass: longhorn
    size: 5Gi
  bootstrap:
    initdb:
      database: glossa
      owner: glossa_owner          # CNPG writes its DSN to glossa-pg-app (key uri)
  managed:
    roles:
      - name: glossa_owner         # runs migrations: CREATEROLE, never superuser
        ensure: present
        login: true
        createrole: true
        superuser: false
      - name: glossa_app           # the server's role: FORCE RLS applies to it
        ensure: present
        login: true
        superuser: false
        bypassrls: false
        createdb: false
        createrole: false
        passwordSecret:
          name: glossa-db-app
```

Create `glossa-db-app` **before** the Cluster: CNPG takes `glossa_app`'s
password from it, and the chart its DSN (`database.app.secretKey`
`DATABASE_URL`), so one Secret holds both:

```sh
PW=$(openssl rand -hex 24)
kubectl -n glossa-platform create secret generic glossa-db-app \
  --type=kubernetes.io/basic-auth \
  --from-literal=username=glossa_app --from-literal=password="$PW" \
  --from-literal=DATABASE_URL="postgres://glossa_app:$PW@glossa-pg-rw:5432/glossa?sslmode=require&pool_max_conns=20"
kubectl -n glossa-platform label secret glossa-db-app cnpg.io/reload=true
```

Then `database.migration.secretName: glossa-pg-app` with `secretKey: uri`
(the owner's DSN, generated by CNPG), and let the server and the
migration Job reach the instances with
`networkPolicy.egress.postgres.to: [{podSelector: {matchLabels:
{cnpg.io/cluster: glossa-pg}}}]`. Backups (Barman to object storage
outside the cluster) are configured on the Cluster, not in this chart.

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
Job is immutable, so it is deleted and re-created per rollout. With
`minio.enabled` the same holds for `<fullname>-minio-bootstrap`, which
runs once MinIO is up (and again whenever a user Secret changes).
`--skip-tests` leaves out the `helm test` Pod. The MinIO StatefulSet's
volumeClaimTemplate is immutable: keep `minio.persistence` unchanged in
the rendered manifests.

## In-namespace MinIO

`minio.enabled` runs one MinIO per product namespace, the Klarlabs
pattern (Brotwerk runs the same pinned image). It is a single-replica,
single-drive StatefulSet; S3 on 9000 is reachable only from glossa-server,
glossa-edge, the bootstrap Job and the test Pod.

**Storage and availability.** glossa-edge serves every published release
from this bucket (it keeps serving what it has cached while MinIO is
down, for `edge.cache.*`), so the delivery plane depends on it. The
default `minio.persistence.storageClassName` is therefore `longhorn`, 3
replicas across the nodes: a node loss moves the pod, and the volume
with it. `longhorn-r1` (1 replica, as Brotwerk uses) is fine for
development. Artifacts can be rebuilt by republishing, but the served
manifests and the release history should survive a node loss, which one
replica does not guarantee. `minio.persistence` is a volumeClaimTemplate
and immutable after install: to grow the volume, edit the PVC
(`data-<fullname>-minio-0`; the Longhorn classes allow expansion) rather than
`minio.persistence.size`. The PVC is retained when the StatefulSet is
deleted or scaled to 0. A PodDisruptionBudget with `maxUnavailable: 1`
lets a node drain evict the pod; `0` blocks drains until it is moved by
hand.

**Credentials.** Nothing is generated by the chart. MinIO's root
credentials come from `minio.rootCredentials.secretName` and reach only
MinIO and the bootstrap Job. The bootstrap Job
(`files/minio-bootstrap.sh`, `quay.io/minio/mc` pinned by digest) runs
after every install and upgrade and is idempotent: it creates the bucket,
the policy `<fullname>-readwrite` (`GetObject`, `PutObject`,
`DeleteObject` and multipart cleanup on `<bucket>/<prefix>/*`) for the
user in `objectStorage.server.secretName`, and `<fullname>-readonly`
(`GetObject` only) for the user in `objectStorage.edge.secretName`. Both
also get `ListBucket` and `GetBucketLocation` on the bucket itself, not
the prefix: without `ListBucket` a missing key answers 403 instead of
404, which glossa counts as a storage failure (the edge's circuit breaker
would open on unknown keys). Rotating a secret key: change
the Secret, `helm upgrade` (the Job updates the user), then restart the
pods reading it. A new *access key* creates a new user; remove the old
one with `mc admin user rm`.

**Transport.** Server and edge talk to MinIO over plain HTTP on the pod
network (`GLOSSA_S3_INSECURE=true`, path-style requests): the traffic
never leaves the namespace and NetworkPolicy admits only them. TLS for
MinIO is not wired into the chart; it would need a certificate for
`<fullname>-minio` and its CA in the server's and edge's trust store.

**Console.** Port 9001 listens inside the pod only:

```sh
kubectl -n glossa-platform port-forward statefulset/<fullname>-minio 9001
# http://localhost:9001, root credentials from minio.rootCredentials
```

**Backups.** Longhorn snapshots and backups cover the volume: label the
PVC for a recurring job group (`minio.persistence.labels`, e.g.
`recurring-job-group.longhorn.io/<group>: enabled`), or back up
`data-<fullname>-minio-0` from the Longhorn UI. A backup target outside
the cluster is what makes it a backup. After a restore, `helm upgrade`
re-runs the bootstrap Job, which is harmless on existing state.

**Images.** MinIO publishes community builds under AGPLv3; the pinned
`quay.io/minio/minio` and `quay.io/minio/mc` digests are the latest tags
on quay.io (September and August 2025). Newer community images are not
published there, so security updates mean building from source or a
different distribution.

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
| server | ingress controller → 8080; `metrics.allowFrom` → 8080 | DNS; `egress.postgres`; object storage¹; `egress.smtp` (SMTP configured); `egress.otlp` (endpoint set) |
| edge | ingress controller → 8081; `metrics.allowFrom` → 8081 | DNS; object storage¹; `egress.otlp` (endpoint set) |
| studio | ingress controller → 8080 | none |
| migrate Job | none | DNS; `egress.postgres` |
| minio | server, edge, bootstrap Job, test Pod → 9000; `extraIngress.minio` | DNS |
| minio-bootstrap Job, minio-test Pod | none | DNS; MinIO → 9000 |

¹ `egress.objectStorage`, or with `minio.enabled` the MinIO pod on 9000
(a podSelector rule; `egress.objectStorage` is then unused).

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
| `objectStorage.endpoint` | REQUIRED; `<fullname>-minio:9000` with `minio.enabled` | `GLOSSA_S3_ENDPOINT`, `host[:port]` without scheme. |
| `objectStorage.bucket` | REQUIRED | `GLOSSA_S3_BUCKET`; the bootstrap Job creates it with `minio.enabled`. |
| `objectStorage.region` | `us-east-1` | `GLOSSA_S3_REGION` (MinIO's default region). |
| `objectStorage.prefix` | `""` | `GLOSSA_S3_PREFIX`; with `minio.enabled` the users' object permissions are scoped to it. |
| `objectStorage.pathStyle` | `false` | `GLOSSA_S3_PATH_STYLE`; always `true` with `minio.enabled`. |
| `objectStorage.insecure` | `false` | `GLOSSA_S3_INSECURE`; always `true` with `minio.enabled` (in-namespace HTTP). |
| `objectStorage.timeout` | `10s` | `GLOSSA_S3_TIMEOUT` |
| `objectStorage.server.secretName` / `.accessKeyIdKey` / `.secretAccessKeyKey` | REQUIRED / `GLOSSA_S3_ACCESS_KEY_ID` / `GLOSSA_S3_SECRET_ACCESS_KEY` | Read-write credentials; with `minio.enabled`, the MinIO user the bootstrap Job creates. |
| `objectStorage.edge.secretName` / keys | `""` → server's / same | Read-only credentials for the edge (recommended; REQUIRED and distinct with `minio.enabled`). |

### In-namespace MinIO

| Value | Default | Meaning |
|---|---|---|
| `minio.enabled` | `false` | MinIO in the release's namespace; objectStorage points at it. |
| `minio.image.repository` / `.tag` / `.digest` | `quay.io/minio/minio` / `RELEASE.2025-09-07T16-13-09Z` / `sha256:14cea4…` | Full image reference (not under `image.registry`). |
| `minio.rootCredentials.secretName` | REQUIRED with `minio.enabled` | Existing Secret; the chart never creates it. |
| `minio.rootCredentials.userKey` / `.passwordKey` | `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | |
| `minio.persistence.storageClassName` | `longhorn` | 3 Longhorn replicas; `longhorn-r1` for development. Immutable after install. |
| `minio.persistence.size` | `5Gi` | Immutable in the template; grow the PVC instead. |
| `minio.persistence.labels` / `.annotations` | `{}` | On the PVC, e.g. a Longhorn recurring-job group. |
| `minio.resources` | 50m / 256Mi, limit 512Mi | |
| `minio.terminationGracePeriodSeconds` | `30` | |
| `minio.pdb.enabled` / `.maxUnavailable` | `true` / `1` | `0` blocks node drains. |
| `minio.nodeSelector` / `.tolerations` / `.affinity` | empty | Also used by the bootstrap Job and the test Pod. |
| `minio.bootstrap.enabled` | `true` | The post-install/post-upgrade Job (and the `helm test` Pod). |
| `minio.bootstrap.image.repository` / `.tag` / `.digest` | `quay.io/minio/mc` / `RELEASE.2025-08-13T08-35-41Z` / `sha256:a7fe34…` | |
| `minio.bootstrap.waitSeconds` | `600` | How long the Job waits for MinIO. |
| `minio.bootstrap.backoffLimit` / `.activeDeadlineSeconds` | `3` / `900` | |
| `minio.bootstrap.resources` | 10m / 32Mi, limit 128Mi | Also the test Pod's. |

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
| `networkPolicy.egress.objectStorage` | `to: []`, 443 | Server and edge; unused with `minio.enabled`. |
| `networkPolicy.egress.smtp` | `to: []`, 587 | Server, only when SMTP is configured. |
| `networkPolicy.egress.otlp` | `to: []`, 4318 | Server/edge, when their OTel endpoint is set. |
| `networkPolicy.extraIngress.{server,edge,studio,minio}` | `[]` | Extra `NetworkPolicyIngressRule`s. |
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
up): `minimal` (external S3, no SMTP), `full` (every optional feature,
external S3 and SMTP) and `minio` (the in-namespace MinIO).
`examples/values-klarlabs.yaml` holds the Klarlabs decisions, with
`TODO(infra)` markers for the questions still open. CI runs lint,
template and kubeconform for every one of them on each pull request that
touches the chart. In a cluster, `helm test <release>` runs the MinIO
check (`files/minio-test.sh`).
