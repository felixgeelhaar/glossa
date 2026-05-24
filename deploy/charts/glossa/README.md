# Glossa Helm chart

Production-leaning Helm chart for [Glossa](https://github.com/felixgeelhaar/glossa). Mirrors the Grafana / cert-manager publishing model — install from the OCI registry at `ghcr.io/felixgeelhaar/charts/glossa`, configure via `values.yaml`, point DNS at the ingress.

## Install

```bash
# Create namespace + secrets first (see "Secrets" section below).
kubectl create namespace glossa
kubectl -n glossa apply -f my-secrets.yaml

# Install the chart from the OCI registry.
helm install glossa oci://ghcr.io/felixgeelhaar/charts/glossa \
  --version 0.1.0 \
  --namespace glossa \
  --values my-values.yaml
```

`helm install` waits for the api + admin Deployments to become ready, then prints the configured ingress hostnames.

## Configuration

The full set of values + their defaults lives in [`values.yaml`](./values.yaml). The fields you'll touch most often:

```yaml
ingress:
  className: traefik
  certManager:
    enabled: true
    clusterIssuer: letsencrypt-prod
  hosts:
    - host: glossa.kraftsport-coach.de
      tls:
        secretName: glossa-kraftsport-coach-de-tls
    - host: glossa.other-tenant.example
      tls:
        secretName: glossa-other-tenant-tls

api:
  replicas: 2
  corsOrigins:
    - https://kraftsport-coach.de
    - https://app.other-tenant.example

postgres:
  mode: bundled         # or "external"
  storage:
    size: 20Gi
    storageClass: longhorn

backup:
  enabled: true
  remoteDestination: "b2:my-bucket/glossa-backups"
```

### Multi-domain

One Glossa instance can sit behind multiple hostnames — listing N hosts mounts the same backend at all of them. Tenants are resolved per-request from the API key (consumer apps) or JWT (admin SPA), so the hostname is purely a routing + branding concern. Each host gets its own cert-manager `Certificate`, its own `IngressRoute`, and an optional `www.<host>` → apex redirect.

### Secrets

The chart expects a pre-created Secret in the install namespace (default name `glossa-app`):

```bash
kubectl -n glossa create secret generic glossa-app \
  --from-literal=DATABASE_URL="postgres://glossa:$(openssl rand -base64 24)@postgres:5432/glossa?sslmode=disable" \
  --from-literal=POSTGRES_USER=glossa \
  --from-literal=POSTGRES_PASSWORD="$(openssl rand -base64 24)" \
  --from-literal=POSTGRES_DB=glossa \
  --from-literal=JWT_SIGNING_KEY="$(openssl rand -base64 48)" \
  --from-literal=GLOSSA_SECRETS_KEY="$(openssl rand -hex 32)" \
  --from-literal=BOOTSTRAP_TENANT_SLUG=demo \
  --from-literal=BOOTSTRAP_TENANT_NAME="Demo Tenant" \
  --from-literal=BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
  --from-literal=BOOTSTRAP_ADMIN_PASSWORD="$(openssl rand -base64 24)"
```

`secrets.create: true` in values asks the chart to render the Secret itself — only for non-prod / staging where committing the values is acceptable.

### External Postgres

Set `postgres.mode: external` to skip the bundled StatefulSet. The chart still expects `DATABASE_URL` in the Secret pointing at whatever you're running (managed Postgres, CrunchyData operator, …).

## Upgrade

```bash
helm upgrade glossa oci://ghcr.io/felixgeelhaar/charts/glossa \
  --version 0.1.1 \
  --namespace glossa \
  --reuse-values \
  --set api.image.tag=v0.1.2 \
  --set admin.image.tag=v0.1.2
```

The migrate initContainer applies any new schema migrations before the api container starts; a bad migration leaves the pod NotReady so the rollout fails closed — `kubectl rollout undo` reverts to the prior release.

## Uninstall

```bash
helm uninstall glossa -n glossa
kubectl delete namespace glossa
```

The bundled Postgres PVC is **not** deleted automatically — `kubectl -n glossa delete pvc data-postgres-0` removes it if you're sure.

## What lives in the chart

| Template | Resource |
|---|---|
| `secret.yaml` | Optional Secret (only when `secrets.create: true`). |
| `postgres.yaml` | StatefulSet + headless Service (bundled mode only). |
| `api.yaml` | Deployment + Service + ConfigMap + migrate initContainer. |
| `admin.yaml` | Deployment + Service. |
| `ingress.yaml` | Per-host `Certificate` + `IngressRoute` + middlewares (compress, www→apex, HTTP→HTTPS). |
| `backup-cronjob.yaml` | Optional daily `pg_dump` + rclone offsite sync. |
| `NOTES.txt` | Post-install pointers. |

## Compatibility

- Kubernetes ≥ 1.28
- Traefik IngressController (k3s ships it; install separately on vanilla k8s)
- cert-manager ≥ 1.13 with a `ClusterIssuer` ready (`letsencrypt-prod` by default)
- Postgres 16 (bundled) or any 14+ if `external`
