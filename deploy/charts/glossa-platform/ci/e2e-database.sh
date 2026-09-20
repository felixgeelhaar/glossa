#!/usr/bin/env bash
# End-to-end check of the chart's database scripts, exactly as rendered
# from ci/postgres-values.yaml, against throwaway containers (Docker):
#
#   Postgres as the StatefulSet runs it (uid 70, read-only root)
#   → postgres-init → the real migrations (glossa-server -migrate=only)
#   → postgres-app-login → the `helm test` script → glossa-server ready
#   → a rotated password → postgres-backup (dump, checks, upload,
#   retention) → the restore drill → the MinIO mirror,
#
# plus the failures those checks exist for: pg_dump 17 against a 16
# server, a dump that does not restore, an empty required table, a stale
# newest dump, an empty bucket replacing the mirror. The rclone remote is
# a local directory (an alias remote named like the Klarlabs one).
#
# Needs docker, helm, go, python3 with PyYAML, openssl and curl. Run from
# anywhere: deploy/charts/glossa-platform/ci/e2e-database.sh
set -euo pipefail

CHART=$(cd "$(dirname "$0")/.." && pwd)
REPO=$(cd "$CHART/../../.." && pwd)
W=$(mktemp -d)
R=$W/rendered
P=gpe2e-$$
PG=docker.io/library/postgres:16.15-alpine@sha256:3c5c8892d184f738f4fe282d14ddaa613a38f00f4189d2d94725ebe6f2909ddb
PG17=docker.io/library/postgres:17-alpine
RC=docker.io/rclone/rclone:1.75.1@sha256:45401ad7410db1d67ffdb58e19059ad20b0d8e0285a60e38bbec55cc1019c7a5
MINIO=quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e
MC=quay.io/minio/mc:RELEASE.2025-08-13T08-35-41Z@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727

step() { printf '\n=== %s\n' "$*"; }
ok() { printf 'e2e ok: %s\n' "$*"; }
die() { printf 'e2e FAIL: %s\n' "$*" >&2; exit 1; }

srv=
cleanup() {
  [[ -z $srv ]] || kill "$srv" 2>/dev/null || true
  docker rm -f $P-postgres $P-minio >/dev/null 2>&1 || true
  # Files the containers wrote belong to their uids: remove them as root.
  docker run --rm --user 0 -v "$W:/w" --entrypoint rm $PG -rf /w/remote >/dev/null 2>&1 || true
  docker volume rm $P-pgdata $P-work $P-work2 >/dev/null 2>&1 || true
  docker network rm $P >/dev/null 2>&1 || true
  rm -rf "$W"
}
trap cleanup EXIT

# ── The scripts and env exactly as rendered (release "t": t-postgres, t-minio).
helm template t "$CHART" -n glossa-platform -f "$CHART/ci/postgres-values.yaml" >"$W/rendered.yaml"
python3 - "$W/rendered.yaml" "$R" <<'PY'
import os, sys, yaml
src, out = sys.argv[1], sys.argv[2]
os.makedirs(out)
for doc in yaml.safe_load_all(open(src)):
    if not doc:
        continue
    spec = doc.get("spec", {})
    pod = {"CronJob": lambda: spec["jobTemplate"]["spec"]["template"]["spec"],
           "Job": lambda: spec["template"]["spec"],
           "Pod": lambda: spec}.get(doc["kind"], lambda: None)()
    for c in (pod or {}).get("initContainers", []) + (pod or {}).get("containers", []):
        if len(c.get("command") or []) == 2 and c["command"][1] == "-c":
            open(os.path.join(out, f"{doc['metadata']['name']}.{c['name']}.sh"), "w").write(c["args"][0])
PY
script() { cat "$R/$1.sh"; }
(cd "$REPO/platform" && go build -o "$W/glossa-server" ./cmd/glossa-server)

SU_PW=$(openssl rand -hex 16)
OWNER_PW=$(openssl rand -hex 16)
APP_PW=$(openssl rand -hex 16)
BACKUP_PW=$(openssl rand -hex 16)

docker network create $P >/dev/null
# Volumes shaped like a pod volume with fsGroup 70: root:70, g+rwxs.
volume() {
  docker volume create "$P-$1" >/dev/null
  docker run --rm --user 0 -v "$P-$1:/d" --entrypoint chown $PG root:70 /d
  docker run --rm --user 0 -v "$P-$1:/d" --entrypoint chmod $PG 2775 /d
}
for v in pgdata work work2; do volume $v; done
RD=$W/remote/db/glossa-platform
MD=$W/remote/glossa-platform/minio
mkdir -p "$RD" "$MD" "$W/rclone"
printf '[storagebox]\ntype = alias\nremote = /remote\n' >"$W/rclone/rclone.conf"
opendirs() { chmod -R a+rwX "$W/remote" 2>/dev/null || true; }
opendirs

# Hardened like the chart's container securityContext.
HARDEN=(--read-only --cap-drop ALL --security-opt no-new-privileges)
libpq() { # user password database
  printf 'PGHOST=t-postgres\nPGPORT=5432\nPGDATABASE=%s\nPGUSER=%s\nPGPASSWORD=%s\nPGCONNECT_TIMEOUT=10\n' "$3" "$1" "$2"
}
bootstrap_env() { # phase
  libpq postgres "$SU_PW" postgres
  printf 'PHASE=%s\nDATABASE=glossa\nOWNER=glossa_owner\nOWNER_PASSWORD=%s\nAPP_PASSWORD=%s\nWAIT_SECONDS=60\n' "$1" "$OWNER_PW" "$APP_PW"
  [[ $1 != init ]] || printf 'BACKUP_USER=glossa_backup\nBACKUP_PASSWORD=%s\n' "$BACKUP_PW"
}
pg_script() { # envfile script [image]
  docker run --rm --network $P --user 70:70 "${HARDEN[@]}" --tmpfs /tmp:uid=70,gid=70 \
    --env-file "$1" --entrypoint /bin/bash "${3:-$PG}" -c "$(script "$2")"
}
psql_as() { # envfile args…
  local env=$1; shift
  docker run --rm --network $P --user 70:70 --env-file "$env" --entrypoint psql $PG -X -q -At -v ON_ERROR_STOP=1 "$@"
}

start_postgres() {
  docker run -d --name $P-postgres --network $P --network-alias t-postgres --user 70:70 "${HARDEN[@]}" \
    --tmpfs /var/run/postgresql:uid=70,gid=70 --tmpfs /tmp:uid=70,gid=70 \
    -v $P-pgdata:/var/lib/postgresql/data -p 127.0.0.1:55432:5432 \
    -e PGDATA=/var/lib/postgresql/data/pgdata -e POSTGRES_USER=postgres \
    -e POSTGRES_PASSWORD="$SU_PW" -e POSTGRES_DB=postgres $PG >/dev/null
  for _ in $(seq 90); do docker exec $P-postgres pg_isready -h 127.0.0.1 -p 5432 >/dev/null 2>&1 && break; sleep 1; done
  docker exec $P-postgres pg_isready -h 127.0.0.1 -p 5432 >/dev/null || die "Postgres is not ready"
}
migrate() {
  MIGRATION_DATABASE_URL=$MIG_DSN DATABASE_URL=$(app_dsn) GLOSSA_AUTH_SECRET=$(openssl rand -base64 32) \
    GLOSSA_LOG_LEVEL=warn "$W/glossa-server" -migrate=only
}
MIG_DSN="host=localhost port=55432 dbname=glossa user=glossa_owner password='$OWNER_PW' sslmode=disable"
app_dsn() { echo "host=localhost port=55432 dbname=glossa user=glossa_app password='$APP_PW' sslmode=disable pool_max_conns=10"; }

step "Postgres as the StatefulSet runs it"
start_postgres
ok "Postgres runs as uid 70 with a read-only root filesystem"

step "postgres-init, twice (idempotent)"
bootstrap_env init >"$W/init.env"
pg_script "$W/init.env" t-postgres-init.bootstrap
pg_script "$W/init.env" t-postgres-init.bootstrap >/dev/null

step "migrations as the owner, on the DSN the chart builds"
migrate
ok "migrations applied"

step "postgres-app-login"
bootstrap_env app-login >"$W/login.env"
pg_script "$W/login.env" t-postgres-app-login.bootstrap

step "helm test: postgres-test"
libpq glossa_app "$APP_PW" glossa >"$W/app.env"
pg_script "$W/app.env" t-postgres-test.test

step "glossa-server starts as glossa_app (its RLS guard passes)"
DATABASE_URL=$(app_dsn) GLOSSA_AUTH_SECRET=$(openssl rand -base64 32) GLOSSA_HTTP_ADDR=127.0.0.1:18080 \
  GLOSSA_LOG_LEVEL=warn "$W/glossa-server" >"$W/server.log" 2>&1 &
srv=$!
for _ in $(seq 30); do curl -fsS -o /dev/null http://127.0.0.1:18080/readyz 2>/dev/null && break; sleep 1; done
curl -fsS -o /dev/null http://127.0.0.1:18080/readyz || { cat "$W/server.log"; die "glossa-server is not ready"; }
kill $srv && wait $srv 2>/dev/null || true
srv=
ok "glossa-server ready"

step "a rotated glossa_app password is applied by the next postgres-init (upgrade)"
APP_PW=$(openssl rand -hex 16)
bootstrap_env init >"$W/init.env"
out=$(pg_script "$W/init.env" t-postgres-init.bootstrap)
[[ $out == *'glossa_app may log in'* ]] || die "no rotation: $out"
libpq glossa_app "$APP_PW" glossa >"$W/app.env"
[[ $(psql_as "$W/app.env" -c 'SELECT current_user') == glossa_app ]] || die "the new password does not log in"
ok "rotated"

step "a tenant, written through row level security as glossa_app"
psql_as "$W/app.env" -c "BEGIN" \
  -c "SELECT set_config('app.tenant_id', 'b9a6c3a2-6f7e-4d1a-9c55-0d3c1f6a7e21', true)" \
  -c "INSERT INTO tenants (id, kind, slug, name) VALUES ('b9a6c3a2-6f7e-4d1a-9c55-0d3c1f6a7e21', 'organization', 'acme', 'Acme')" \
  -c "COMMIT" >/dev/null

step "the owner cannot pg_dump (FORCE ROW LEVEL SECURITY), hence the backup role"
libpq glossa_owner "$OWNER_PW" glossa >"$W/owner.env"
if docker run --rm --network $P --user 70:70 --env-file "$W/owner.env" --entrypoint pg_dump $PG >/dev/null 2>"$W/owner.err"; then
  die "the owner could dump"
fi
grep -q 'row-level security' "$W/owner.err" || die "unexpected: $(cat "$W/owner.err")"
ok "refused: $(grep -o 'query would be affected by row-level security policy for table "[^"]*"' "$W/owner.err" | head -1)"

# Retention fixtures: our old dump, a foreign old file, our old dump one level down.
touch -t 202601010000 "$RD/glossa-20260101T034000Z.sql.gz" "$RD/brotwerk-20260101T030000Z.sql.gz"
mkdir -p "$RD/archive" && touch -t 202601010000 "$RD/archive/glossa-20260101T034000Z.sql.gz"
opendirs

backup() { # [image of the dump container]
  { libpq glossa_backup "$BACKUP_PW" glossa; printf 'OUT_DIR=/work\nPREFIX=glossa\n'; } >"$W/dump.env"
  docker run --rm --network $P --user 70:70 "${HARDEN[@]}" --tmpfs /tmp:uid=70,gid=70 \
    -v $P-work:/work --env-file "$W/dump.env" --entrypoint /bin/bash "${1:-$PG}" -c "$(script t-postgres-backup.dump)" || return 1
  docker run --rm --user 70:70 "${HARDEN[@]}" --tmpfs /tmp:uid=70,gid=70 \
    -v $P-work:/work:ro -v "$W/rclone:/etc/rclone:ro" -v "$W/remote:/remote" \
    -e RCLONE_CONFIG=/etc/rclone/rclone.conf -e RCLONE_CACHE_DIR=/tmp/rclone-cache -e HOME=/tmp \
    -e IN_DIR=/work -e REMOTE=storagebox:db/glossa-platform -e PREFIX=glossa -e RETENTION_DAYS=14 \
    --entrypoint /bin/sh $RC -c "$(script t-postgres-backup.upload)"
}

step "postgres-backup: dump, integrity checks, upload, retention"
backup
opendirs
[[ ! -e $RD/glossa-20260101T034000Z.sql.gz ]] || die "retention kept our old dump"
[[ -e $RD/brotwerk-20260101T030000Z.sql.gz ]] || die "retention deleted a foreign file"
[[ -e $RD/archive/glossa-20260101T034000Z.sql.gz ]] || die "retention reached into a subdirectory"
ok "retention removed only this release's old dumps, in this directory only"

step "postgres-backup refuses pg_dump 17 against the 16 server"
if backup $PG17 2>"$W/dump17.err"; then die "a pg_dump 17 dump was accepted"; fi
grep -q 'pg_dump 17 against a Postgres 16 server' "$W/dump17.err" || die "unexpected: $(cat "$W/dump17.err")"
ok "major-version guard"

restore() { # max-age-hours non-empty-tables
  docker run --rm --user 70:70 "${HARDEN[@]}" --tmpfs /tmp:uid=70,gid=70 \
    -v $P-work2:/work -v "$W/rclone:/etc/rclone:ro" -v "$W/remote:/remote:ro" \
    -e RCLONE_CONFIG=/etc/rclone/rclone.conf -e RCLONE_CACHE_DIR=/tmp/rclone-cache -e HOME=/tmp \
    -e OUT_DIR=/work -e REMOTE=storagebox:db/glossa-platform -e PREFIX=glossa -e MAX_AGE_HOURS="$1" \
    --entrypoint /bin/sh $RC -c "$(script t-postgres-restore-test.fetch)" || return 1
  docker run --rm --user 70:70 "${HARDEN[@]}" --tmpfs /tmp:uid=70,gid=70 --tmpfs /var/run/postgresql:uid=70,gid=70 \
    -v $P-work2:/work -e IN_DIR=/work -e OWNER=glossa_owner -e "ROLES=glossa_owner glossa_app glossa_system" \
    -e NON_EMPTY_TABLES="$2" --entrypoint /bin/bash $PG -c "$(script t-postgres-restore-test.restore)"
}

step "postgres-restore-test: the newest dump into a throwaway Postgres"
restore 48 tenants || die "the restore drill failed"
ok "restore drill"

step "the drill fails on an empty required table"
if restore 48 "tenants identity_members" >"$W/empty.log" 2>&1; then die "an empty required table passed"; fi
grep -q 'identity_members came back empty' "$W/empty.log" || die "unexpected: $(tail -3 "$W/empty.log")"
ok "caught"

step "the drill fails on a pg_dump 17 dump (v0.3's unrestorable dumps)"
newest="glossa-29991231T235959Z.sql.gz" # sorts last; a future timestamp passes the age check
docker run --rm --network $P --env-file "$W/dump.env" --entrypoint pg_dump $PG17 | gzip >"$RD/$newest"
if restore 48 tenants >"$W/restore17.log" 2>&1; then die "a pg_dump 17 dump restored"; fi
grep -q 'transaction_timeout' "$W/restore17.log" || die "unexpected: $(tail -3 "$W/restore17.log")"
rm "$RD/$newest"
ok "caught: $(grep -o 'unrecognized configuration parameter "transaction_timeout"' "$W/restore17.log" | head -1)"

step "the drill fails when the newest dump is stale"
for f in "$RD"/glossa-2*.sql.gz; do mv "$f" "$RD/glossa-20260102T034000Z.sql.gz"; done
if restore 48 "" >"$W/stale.log" 2>&1; then die "a stale dump passed"; fi
grep -q 'h old (limit 48h)' "$W/stale.log" || die "unexpected: $(tail -3 "$W/stale.log")"
ok "caught"

step "README runbook: restore the newest dump onto a new, empty volume"
docker rm -f $P-postgres >/dev/null
docker volume rm $P-pgdata >/dev/null && volume pgdata
start_postgres
pg_script "$W/init.env" t-postgres-init.bootstrap >/dev/null # helm upgrade: postgres-init
psql_su() { docker exec -i $P-postgres psql -U postgres -v ON_ERROR_STOP=1 "$@"; }
psql_su -c 'DROP DATABASE glossa WITH (FORCE)' -c 'CREATE DATABASE glossa OWNER glossa_owner' >/dev/null
psql_su -c 'SET ROLE glossa_owner' \
  -c 'CREATE ROLE glossa_app NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS' \
  -c 'CREATE ROLE glossa_system NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS' \
  -c 'GRANT glossa_system TO glossa_app WITH INHERIT FALSE, SET TRUE' >/dev/null
gunzip -c "$RD/glossa-20260102T034000Z.sql.gz" | psql_su -d glossa --single-transaction -q >/dev/null
migrate                                                            # helm upgrade: migrate (nothing new)
bootstrap_env app-login >"$W/login.env"                            # the rotated password
pg_script "$W/login.env" t-postgres-app-login.bootstrap >/dev/null # helm upgrade: postgres-app-login
pg_script "$W/app.env" t-postgres-test.test >/dev/null
[[ $(psql_su -At -d glossa -c 'SELECT count(*) FROM tenants') == 1 ]] || die "the tenant did not come back"
ok "restored onto a new volume; glossa_app passes the helm test; the tenant is back"

step "minio-backup: mirror, deleted/ retention, empty-source guard"
docker run -d --name $P-minio --network $P --network-alias t-minio --user 1000:1000 \
  --tmpfs /data:uid=1000,gid=1000 --tmpfs /tmp:uid=1000,gid=1000 \
  -e MINIO_ROOT_USER=glossa-root -e MINIO_ROOT_PASSWORD=rootpassword123 \
  $MINIO server /data --address :9000 --certs-dir /tmp/certs >/dev/null
MC_ENV=(-e MINIO_URL=http://t-minio:9000 -e BUCKET=glossa -e OBJECT_PREFIX= -e MC_CONFIG_DIR=/tmp/.mc -e HOME=/tmp
  -e RW_ACCESS_KEY=glossa-server -e RW_SECRET_KEY=serversecret123 -e RO_ACCESS_KEY=glossa-edge -e RO_SECRET_KEY=edgesecret12345)
docker run --rm --network $P --user 1000:1000 --tmpfs /tmp:uid=1000,gid=1000 "${MC_ENV[@]}" \
  -e MINIO_ROOT_USER=glossa-root -e MINIO_ROOT_PASSWORD=rootpassword123 -e POLICY_RW=t-readwrite -e POLICY_RO=t-readonly \
  -e WAIT_SECONDS=60 --entrypoint /bin/bash $MC -c "$(script t-minio-bootstrap.bootstrap)" >/dev/null
mc_rw() { docker run --rm -i --network $P --user 1000:1000 --tmpfs /tmp:uid=1000,gid=1000 -e MC_CONFIG_DIR=/tmp/.mc \
  -e MC_HOST_s=http://glossa-server:serversecret123@t-minio:9000 --entrypoint mc $MC "$@" >/dev/null; }
echo '{"v":1}' | mc_rw pipe -q s/glossa/t1/production/manifest.json
echo '{"a":1}' | mc_rw pipe -q s/glossa/t1/artifacts/a.json
mkdir -p "$MD/deleted/20260101T000000Z" "$MD/deleted/keep-me" && touch "$MD/deleted/20260101T000000Z/old"
opendirs
mirror() {
  docker run --rm --network $P --user 65532:65532 "${HARDEN[@]}" --tmpfs /tmp:uid=65532,gid=65532 \
    -v "$W/rclone:/etc/rclone:ro" -v "$W/remote:/remote" \
    -e RCLONE_CONFIG=/etc/rclone/rclone.conf -e RCLONE_CACHE_DIR=/tmp/rclone-cache -e HOME=/tmp \
    -e RCLONE_CONFIG_MINIO_TYPE=s3 -e RCLONE_CONFIG_MINIO_PROVIDER=Minio -e RCLONE_CONFIG_MINIO_ENDPOINT=http://t-minio:9000 \
    -e RCLONE_CONFIG_MINIO_REGION=us-east-1 -e RCLONE_CONFIG_MINIO_FORCE_PATH_STYLE=true \
    -e RCLONE_CONFIG_MINIO_ACCESS_KEY_ID=glossa-edge -e RCLONE_CONFIG_MINIO_SECRET_ACCESS_KEY=edgesecret12345 \
    -e BUCKET=glossa -e OBJECT_PREFIX= -e REMOTE=storagebox:glossa-platform/minio -e DELETED_RETENTION_DAYS=30 \
    --entrypoint /bin/sh $RC -c "$(script t-minio-backup.mirror)"
}
mirror
opendirs
[[ -f $MD/current/t1/production/manifest.json && -f $MD/current/t1/artifacts/a.json ]] || die "the mirror is incomplete"
[[ ! -e $MD/deleted/20260101T000000Z ]] || die "an expired deleted/ run was kept"
[[ -e $MD/deleted/keep-me ]] || die "a foreign directory under deleted/ was removed"
ok "mirrored with glossa-edge's read-only user"
mc_rw rm -q s/glossa/t1/artifacts/a.json
mirror
opendirs
[[ -n $(find "$MD/deleted" -name a.json) ]] || die "the deleted object is not in deleted/"
[[ ! -e $MD/current/t1/artifacts/a.json ]] || die "the deleted object is still in current/"
ok "a deleted object moved to deleted/<timestamp>"
mc_rw rm -q s/glossa/t1/production/manifest.json
if mirror 2>"$W/empty.err"; then die "an empty bucket replaced the mirror"; fi
grep -q 'refusing to empty it' "$W/empty.err" || die "unexpected: $(cat "$W/empty.err")"
[[ -f $MD/current/t1/production/manifest.json ]] || die "the mirror was emptied"
ok "an empty source is refused"

printf '\nall database e2e checks passed\n'
