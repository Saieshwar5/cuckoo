#!/usr/bin/env bash
#
# backup.sh — runs ON THE SERVER, from cron, once a night:
#
#   17 3 * * * /srv/cuckoo/backup.sh >> /var/log/cuckoo-backup.log 2>&1
#
# A database dump, kept locally for two weeks, and a copy of the dumps and the
# media folder to the bucket named by BACKUP_REMOTE in .env (an rclone remote,
# e.g. spaces:cuckoo-backups). Without BACKUP_REMOTE it dumps locally and says
# so, which is better than nothing and worse than what you want.
#
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
set -a; . ./.env; set +a

DATA_DIR="${DATA_DIR:-/data}"
BACKUPS="$DATA_DIR/backups"
mkdir -p "$BACKUPS"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dump="$BACKUPS/cuckoo-$stamp.sql.gz"
docker compose exec -T postgres pg_dump -U cuckoo --no-owner cuckoo | gzip > "$dump"
echo "$(date -u +%FT%TZ) dumped $(du -h "$dump" | cut -f1) to $dump"

find "$BACKUPS" -name 'cuckoo-*.sql.gz' -mtime +14 -delete

if [[ -n "${BACKUP_REMOTE:-}" ]] && command -v rclone >/dev/null 2>&1; then
  rclone copy --quiet "$BACKUPS" "$BACKUP_REMOTE/backups"
  rclone sync --quiet "$DATA_DIR/media" "$BACKUP_REMOTE/media"
  echo "$(date -u +%FT%TZ) copied to $BACKUP_REMOTE"
else
  echo "$(date -u +%FT%TZ) BACKUP_REMOTE not set or rclone missing: local dump only" >&2
fi
