#!/usr/bin/env bash
#
# deploy.sh — build the images and the site on this machine, ship them to the
# server, and start them. Nothing is compiled on the server.
#
#   deploy/deploy.sh                    build, ship, start
#   deploy/deploy.sh rollback <tag>     put an earlier image tag back
#
# Needs: DEPLOY_HOST (user@host), and SITE_DOMAIN unless deploy/.env exists
# locally with it. Passwordless ssh to the host. rsync on both ends. Nothing
# here uses sudo: the deploy user owns /srv/cuckoo and /srv/www and is in the
# docker group (deploy/README.md, "The instance").
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

HOST="${DEPLOY_HOST:?set DEPLOY_HOST, e.g. ubuntu@203.0.113.4}"
REMOTE_DIR="${REMOTE_DIR:-/srv/cuckoo}"
WWW_DIR="${REMOTE_WWW_DIR:-/srv/www}"

if [[ -z "${SITE_DOMAIN:-}" && -f deploy/.env ]]; then
  SITE_DOMAIN="$(sed -n 's/^SITE_DOMAIN=//p' deploy/.env | head -1)"
fi
: "${SITE_DOMAIN:?set SITE_DOMAIN (or put it in deploy/.env)}"

remote() { ssh -o BatchMode=yes "$HOST" "$@"; }

set_tags() {
  local v="$1"
  remote "cd '$REMOTE_DIR' && \
    grep -q '^HUB_IMAGE=' .env || echo 'HUB_IMAGE=' >> .env; \
    grep -q '^WELCOME_IMAGE=' .env || echo 'WELCOME_IMAGE=' >> .env; \
    sed -i -E 's|^HUB_IMAGE=.*|HUB_IMAGE=cuckoo-hub:$v|; s|^WELCOME_IMAGE=.*|WELCOME_IMAGE=cuckoo-welcome:$v|' .env"
}

if [[ "${1:-}" == "rollback" ]]; then
  TAG="${2:?rollback needs the tag to go back to, e.g. abc1234}"
  echo "rolling back to $TAG"
  set_tags "$TAG"
  remote "cd '$REMOTE_DIR' && docker compose up -d --remove-orphans"
  curl -fsS "https://$SITE_DOMAIN/healthz"; echo
  exit 0
fi

VERSION="$(git describe --always --dirty)"
if [[ "$VERSION" == *-dirty ]]; then
  echo "warning: uncommitted changes; deploying as $VERSION" >&2
fi

echo "==> building images ($VERSION)"
docker build -q -t "cuckoo-hub:$VERSION" --build-arg "VERSION=$VERSION" server/
docker build -q -t "cuckoo-welcome:$VERSION" -f examples/welcome/Dockerfile .

echo "==> building the site for https://$SITE_DOMAIN"
(cd web && npm ci --no-audit --no-fund --silent && SITE_URL="https://$SITE_DOMAIN" npm run build --silent)

echo "==> shipping images"
docker save "cuckoo-hub:$VERSION" "cuckoo-welcome:$VERSION" | gzip | remote 'gunzip | docker load'

echo "==> shipping files"
remote "mkdir -p '$REMOTE_DIR' '$WWW_DIR'"
scp -q deploy/compose.yml deploy/Caddyfile deploy/backup.sh "$HOST:$REMOTE_DIR/"
remote "chmod +x '$REMOTE_DIR/backup.sh'"
rsync -az --delete web/dist/ "$HOST:$WWW_DIR/"

echo "==> starting"
set_tags "$VERSION"
remote "cd '$REMOTE_DIR' && docker compose up -d --remove-orphans"

echo "==> checking"
for _ in $(seq 1 20); do
  if out=$(curl -fsS "https://$SITE_DOMAIN/healthz" 2>/dev/null); then
    echo "$out"; echo "deployed $VERSION"; exit 0
  fi
  sleep 3
done
echo "the hub did not answer on https://$SITE_DOMAIN/healthz; see: ssh $HOST 'cd $REMOTE_DIR && docker compose logs --tail 100 hub'" >&2
exit 1
