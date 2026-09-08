#!/usr/bin/env bash
#
# backup.sh — runs ON THE SERVER, from cron, once a night:
#
#   17 3 * * * /srv/cuckoo/backup.sh >> /var/log/cuckoo-backup.log 2>&1
#
# A dump of the database, kept locally for two weeks and copied to
# BACKUP_BUCKET. Both the dump and the copy run in throwaway containers, so
# the machine needs neither a Postgres client nor the AWS CLI installed —
# the same rule as everything else here.
#
# RDS takes its own snapshots, and they are not this. A snapshot restores an
# instance; a dump can be read, searched, restored into a scratch database on
# a laptop, and loaded into a Postgres that is not RDS at all. Keep both.
#
# Uploaded files are in S3 with versioning on, so nothing here copies them.
# With CUCKOO_BLOBS=disk they are under DATA_DIR on this machine instead, and
# then they are synced too.
#
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
set -a; . ./.env; set +a

DATA_DIR="${DATA_DIR:-/data}"
BACKUPS="$DATA_DIR/backups"
mkdir -p "$BACKUPS"

: "${DB_HOST:?set DB_HOST in .env}"
: "${DB_PASSWORD:?set DB_PASSWORD in .env}"
DB_USER="${DB_USER:-cuckoo}"
DB_NAME="${DB_NAME:-cuckoo}"
DB_PORT="${DB_PORT:-5432}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dump="$BACKUPS/cuckoo-$stamp.sql.gz"

docker run --rm -e PGPASSWORD="$DB_PASSWORD" postgres:16-alpine \
  pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" --no-owner "$DB_NAME" \
  | gzip > "$dump"
echo "$(date -u +%FT%TZ) dumped $(du -h "$dump" | cut -f1) to $dump"

find "$BACKUPS" -name 'cuckoo-*.sql.gz' -mtime +14 -delete

if [[ -z "${BACKUP_BUCKET:-}" ]]; then
  echo "$(date -u +%FT%TZ) BACKUP_BUCKET not set: local dump only" >&2
  exit 0
fi

# --network host so the container reaches the instance metadata service for
# the IAM role; there is no access key to pass in.
aws_cli() {
  docker run --rm --network host \
    -e AWS_REGION -e AWS_DEFAULT_REGION="${AWS_REGION:-}" \
    -v "$DATA_DIR:$DATA_DIR:ro" \
    amazon/aws-cli:latest "$@"
}

aws_cli s3 sync "$BACKUPS" "$BACKUP_BUCKET/backups" --only-show-errors
echo "$(date -u +%FT%TZ) copied dumps to $BACKUP_BUCKET/backups"

if [[ "${CUCKOO_BLOBS:-s3}" == "disk" ]]; then
  aws_cli s3 sync "$DATA_DIR/media" "$BACKUP_BUCKET/media" --only-show-errors
  echo "$(date -u +%FT%TZ) copied media to $BACKUP_BUCKET/media"
fi
