#!/bin/sh
# Nightly mirror of the release bucket (<fullname>-minio-backup, rclone
# image, busybox sh). The Brotwerk pattern (mc mirror, then rclone sync to
# the Storage Box) in one step: rclone reads MinIO directly with
# glossa-edge's read-only credentials, so there is no scratch copy of the
# bucket and no root.
#
#   <REMOTE>/current/             the bucket (prefix) as of this run
#   <REMOTE>/deleted/<timestamp>/ what this run removed or overwrote there,
#                                 kept DELETED_RETENTION_DAYS days
#
# A plain sync would carry an accidental delete (or an empty bucket on a
# freshly lost volume) into the backup the next night; --backup-dir keeps
# those objects instead, and an empty source never replaces a non-empty
# mirror.
#
# Environment (set by the chart):
#   RCLONE_CONFIG, RCLONE_CONFIG_MINIO_*  rclone.conf and the MinIO remote
#   BUCKET, OBJECT_PREFIX                 the source (prefix may be empty)
#   REMOTE                                <remote>:<directory>, never a remote's root
#   DELETED_RETENTION_DAYS
set -eu

log() { printf 'minio-backup: %s\n' "$*"; }
fail() { printf 'minio-backup: ERROR: %s\n' "$*" >&2; exit 1; }
count() { rclone size --json "$1" 2>/dev/null | sed -n 's/.*"count":\([0-9]*\).*/\1/p'; }

case $DELETED_RETENTION_DAYS in '' | *[!0-9]* | 0) fail "DELETED_RETENTION_DAYS must be a positive integer" ;; esac
src="minio:$BUCKET${OBJECT_PREFIX:+/$OBJECT_PREFIX}"
ts=$(date -u +%Y%m%dT%H%M%SZ)

src_count=$(count "$src")
[ -n "$src_count" ] || fail "cannot list $src"
dst_count=$(count "$REMOTE/current")
if [ "$src_count" -eq 0 ] && [ "${dst_count:-0}" -gt 0 ]; then
  fail "$src is empty but the mirror holds $dst_count objects; refusing to empty it (restore MinIO from $REMOTE/current, or clear the mirror by hand)"
fi

rclone sync "$src" "$REMOTE/current" --backup-dir "$REMOTE/deleted/$ts"
log "mirrored $src_count objects from $src to $REMOTE/current"

# Retention by the run's timestamp: objects moved to deleted/ keep their
# original modification time, so --min-age would drop them at once.
cutoff=$(date -u -d "@$(($(date +%s) - DELETED_RETENTION_DAYS * 86400))" +%Y%m%d%H%M%S)
rclone lsf --dirs-only --max-depth 1 "$REMOTE/deleted/" 2>/dev/null | sed 's#/$##' |
  while IFS= read -r run; do
    case $run in
    [0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]T[0-9][0-9][0-9][0-9][0-9][0-9]Z) ;;
    *) continue ;;
    esac
    if [ "$(printf '%s' "$run" | tr -d TZ)" -lt "$cutoff" ]; then
      rclone purge "$REMOTE/deleted/$run"
      log "removed deleted/$run (older than ${DELETED_RETENTION_DAYS}d)"
    fi
  done
