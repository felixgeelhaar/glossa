# Glossa — k3s deployment

Reference manifests for self-hosting Glossa on a k3s cluster behind Traefik. Mirrors the layout the public `ghcr.io/felixgeelhaar/glossa-*` container images expect.

## Prerequisites

- k3s (or any Kubernetes ≥ 1.28) with the Traefik IngressController bundled by k3s.
- `cert-manager` installed in-cluster with a `letsencrypt-prod` `ClusterIssuer` (flip to `letsencrypt-staging` in `ingress.yaml` until cert issuance is green to avoid burning Let's Encrypt prod quota).
- DNS: `glossa.kraftsport-coach.de` A record pointing at the node's public IP. `www.glossa.kraftsport-coach.de` optional — `ingress.yaml` 308-redirects it to the apex.
- An `ssh-host-keys` ConfigMap if you enable `backup-cronjob.yaml` (rclone offsite backup).

## One-time deploy

```bash
# 1. Generate fresh secrets locally — do NOT reuse the example values
cp deploy/k3s/glossa/secrets.example.yaml /tmp/glossa-secrets.yaml
# Replace REPLACE_ME values:
openssl rand -base64 48   # → JWT_SIGNING_KEY
openssl rand -hex 32      # → GLOSSA_SECRETS_KEY
openssl rand -base64 24   # → POSTGRES_PASSWORD + DATABASE_URL embedded copy
# (paste into /tmp/glossa-secrets.yaml)

# 2. Apply secrets first so the deployments can pick them up
kubectl apply -f /tmp/glossa-secrets.yaml
shred -u /tmp/glossa-secrets.yaml

# 3. Apply the rest of the stack
kubectl apply -k deploy/k3s/glossa/

# 4. Watch the rollout — migrate initContainer runs first, then api,
#    then admin. Cert-manager picks up the Certificate resource and
#    requests a Let's Encrypt cert (~30-90s).
kubectl -n glossa rollout status deploy/api
kubectl -n glossa rollout status deploy/admin
```

After the rollout settles, the admin SPA is live at <https://glossa.kraftsport-coach.de> and the API at <https://glossa.kraftsport-coach.de/api/v1>.

## Upgrade to a new image tag

```bash
TAG=v0.1.2
kubectl -n glossa set image deploy/api \
  api=ghcr.io/felixgeelhaar/glossa-api:$TAG \
  migrate=ghcr.io/felixgeelhaar/glossa-api:$TAG
kubectl -n glossa set image deploy/admin admin=ghcr.io/felixgeelhaar/glossa-admin:$TAG
kubectl -n glossa rollout status deploy/api  --timeout=180s
kubectl -n glossa rollout status deploy/admin --timeout=120s
# Rollback if the rollout fails:
# kubectl -n glossa rollout undo deploy/api && kubectl -n glossa rollout undo deploy/admin
```

The `migrate` initContainer reuses the api image — bumping both keeps the schema migration locked to the api version.

## What lives where

| File | Resource |
|---|---|
| `postgres.yaml` | Postgres 16 StatefulSet + PVC + headless Service `postgres`. |
| `api.yaml` | Go API Deployment + Service `api:8080` + `glossa-app-config` ConfigMap + `migrate` initContainer. |
| `admin.yaml` | Nginx Deployment serving the SPA bundle + Service `admin:80`. |
| `ingress.yaml` | Traefik IngressRoute (`/api/*` → api, everything else → admin), cert-manager `Certificate`, www→apex redirect, HTTP→HTTPS redirect. |
| `backup-cronjob.yaml` | Optional daily Postgres dump → rclone-backed offsite target. |
| `secrets.example.yaml` | Template; do not apply as-is. |

## Tenancy

The first user logs in with the bootstrap admin credentials. Create extra tenants from the admin SPA → "Projects" once you're in. Each tenant gets isolated rows via the Postgres RLS policies in the schema (`SET LOCAL app.current_tenant`).

## Backups

`backup-cronjob.yaml` runs `pg_dump` every 24h and rclone-syncs the dump to an offsite target. To enable, also apply the `rclone-config` secret referenced by the CronJob (`rclone.conf` + matching SSH key). Disable by removing the CronJob from `kustomization.yaml`.

## Pulling private images

GHCR images are public — no `imagePullSecrets` needed.
