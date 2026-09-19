#!/usr/bin/env bash
# The first half of the nightly Postgres backup (the init container
# "dump" of <fullname>-postgres-backup, postgres image): a plain-SQL
# pg_dump of the application database, gzipped into the Job's scratch
# volume and verified before the "upload" container ships it. As in v0.3
# (deploy/k3s/glossa/backup-cronjob.yaml), what gets uploaded is checked
# first: a pg_dump that dies after the header still exits 0 through a
# pipe, and a job that ships 20 bytes every night reports success until
# the day it is needed.
#
# Environment (set by the chart):
#   PGHOST, PGPORT, PGDATABASE, PGUSER, PGPASSWORD   the backup role's connection
#   OUT_DIR     the scratch volume shared with the upload container
#   PREFIX      dump file prefix: <PREFIX>-<UTC timestamp>.sql.gz
set -euo pipefail

log() { printf 'postgres-dump: %s\n' "$*"; }
fail() { printf 'postgres-dump: ERROR: %s\n' "$*" >&2; exit 1; }

# 1. pg_dump must be the server's major version. A newer pg_dump writes
#    settings an older server cannot parse (pg_dump 17 emits
#    `SET transaction_timeout`, unknown to 16), so its dump restores ZERO
#    tables under ON_ERROR_STOP. v0.3 shipped exactly that for weeks: the
#    dumps looked perfect from this side (2026-09-13, commit ace78d5).
client=$(pg_dump --version | sed -E 's/^[^0-9]*([0-9]+).*/\1/')
server=$(psql -X -At -v ON_ERROR_STOP=1 -c 'SHOW server_version_num')
server=$((server / 10000))
[[ $client == "$server" ]] ||
  fail "pg_dump $client against a Postgres $server server; postgres.image must be the server's major version"
log "pg_dump $client, server $server"

ts=$(date -u +%Y%m%dT%H%M%SZ)
name="$PREFIX-$ts.sql.gz"
partial="$OUT_DIR/$name.partial"

# 2. Dump. pipefail makes a failing pg_dump fail the pipe.
pg_dump --no-password | gzip -6 >"$partial"

# 3. Verify: an intact archive, pg_dump's own completion trailer (absent
#    when it died midway), and one CREATE TABLE per table of the live
#    database (a dump without schema is worthless however well it gzips).
gzip -t "$partial" || fail "the archive is corrupt"
gunzip -c "$partial" | tail -n 5 | grep -q '^-- PostgreSQL database dump complete' ||
  fail "the dump is truncated (no completion trailer)"
ddl=$(gunzip -c "$partial" | grep -cE '^CREATE (UNLOGGED )?TABLE ' || true)
live=$(psql -X -At -v ON_ERROR_STOP=1 -c \
  "SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog', 'information_schema')")
((ddl >= 1)) || fail "the dump has no CREATE TABLE statements"
((ddl == live)) || fail "the dump has $ddl CREATE TABLE statements, the database $live tables"

mv "$partial" "$OUT_DIR/$name"
printf '%s\n' "$name" >"$OUT_DIR/dump.name"
log "$name: $(stat -c %s "$OUT_DIR/$name") bytes, $ddl tables"
