# Glossa k3s deploy

Single-node k3s on `edge-1` (Hetzner CX33), mirroring the
Brotwerk/IRI pattern. One namespace `glossa`. One binary API, one
nginx-served admin SPA, one Postgres StatefulSet, Traefik
ingress, cert-manager TLS, daily backups to a Hetzner Storage Box.

## One-time setup

```bash
# Namespace + secrets first. Edit values, then apply.
cp secrets.example.yaml secrets.yaml
# ... edit DATABASE_URL, JWT_SIGNING_KEY, POSTGRES_PASSWORD ...
kubectl apply -f secrets.yaml

# Backup target — create the SFTP key and rclone config locally,
# then push as a Secret (see backup-cronjob.yaml header).
kubectl -n glossa create secret generic rclone-config \
  --from-file=rclone.conf=$HOME/.config/rclone/rclone.conf \
  --from-file=id_storagebox=$HOME/.ssh/id_storagebox

# Bring the stack up.
kubectl apply -k deploy/k3s/glossa
```

## Release flow

`git tag v0.1.0 && git push --tags` triggers
`.github/workflows/release.yml`. That job:

1. Builds + pushes `ghcr.io/felixgeelhaar/glossa-api:<tag>` and
   `glossa-admin:<tag>`.
2. SSHes to `edge-1` over Tailscale and runs
   `kubectl -n glossa set image deploy/api ...` for the new tag,
   then the same for `deploy/admin`.
3. Waits for `kubectl rollout status` and auto-rolls-back via
   `kubectl rollout undo` if the new pod doesn't become Ready
   (covers bad migrations — the `migrate` initContainer fails
   visibly).
4. HTTP smoke-tests `https://glossa.app/api/healthz` and
   `https://glossa.app/`; on failure, also auto-rolls-back.

## Manual rollback

```bash
ssh edge-1 'kubectl -n glossa rollout undo deploy/api'
ssh edge-1 'kubectl -n glossa rollout undo deploy/admin'
```

If a schema change also needs reverting:

```bash
kubectl -n glossa exec deploy/api -- /migrate \
  -path /migrations -database "$DATABASE_URL" down 1
```

## Restore from backup

```bash
# Pull the latest dump locally.
rclone copy storagebox:db/glossa-LATEST.sql.gz .

# Pipe into the running pod's psql.
gunzip -c glossa-LATEST.sql.gz | \
  kubectl -n glossa exec -i postgres-0 -- \
  psql -U glossa glossa
```
