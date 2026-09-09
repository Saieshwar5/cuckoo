#!/usr/bin/env bash
#
# deploy-runtime.sh — build the runtime image here, ship it, start it.
# Nothing is compiled on the server.
#
#   deploy/deploy-runtime.sh                  build, ship, start
#   deploy/deploy-runtime.sh rollback <tag>   put an earlier image tag back
#
# Needs: RUNTIME_DEPLOY_HOST (user@host) and RUNTIME_DOMAIN, or deploy/.env
# with them. Passwordless ssh to the host, which owns /srv/runtime and is in
# the docker group.
#
# The runtime keeps its own database and its own instance (D69): a slow model
# afternoon must not take the hub's CPU with it.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -f deploy/.env ]]; then
  RUNTIME_DEPLOY_HOST="${RUNTIME_DEPLOY_HOST:-$(sed -n 's/^RUNTIME_DEPLOY_HOST=//p' deploy/.env | head -1)}"
  RUNTIME_DOMAIN="${RUNTIME_DOMAIN:-$(sed -n 's/^RUNTIME_DOMAIN=//p' deploy/.env | head -1)}"
fi
HOST="${RUNTIME_DEPLOY_HOST:?set RUNTIME_DEPLOY_HOST, e.g. admin@13.200.0.1}"
: "${RUNTIME_DOMAIN:?set RUNTIME_DOMAIN, e.g. runtime.cuckoo.onl}"
REMOTE_DIR="${RUNTIME_REMOTE_DIR:-/srv/runtime}"

remote() { ssh -o BatchMode=yes "$HOST" "$@"; }

set_tag() {
  remote "cd '$REMOTE_DIR' && \
    grep -q '^RUNTIME_IMAGE=' .env || echo 'RUNTIME_IMAGE=' >> .env; \
    sed -i -E 's|^RUNTIME_IMAGE=.*|RUNTIME_IMAGE=cuckoo-runtime:$1|' .env"
}

if [[ "${1:-}" == "rollback" ]]; then
  TAG="${2:?rollback needs the tag to go back to, e.g. abc1234}"
  echo "rolling back to $TAG"
  set_tag "$TAG"
  remote "cd '$REMOTE_DIR' && docker compose -f runtime-compose.yml up -d --remove-orphans"
  curl -fsS "https://$RUNTIME_DOMAIN/healthz"; echo
  exit 0
fi

VERSION="$(git describe --always --dirty)"
if [[ "$VERSION" == *-dirty ]]; then
  echo "warning: uncommitted changes; deploying as $VERSION" >&2
fi

# The image is built from the repository root: the runtime depends on
# sdk/typescript by path, so the build needs both.
echo "==> building the image ($VERSION)"
docker build -q -f runtime/Dockerfile -t "cuckoo-runtime:$VERSION" --build-arg "VERSION=$VERSION" .

echo "==> shipping the image"
docker save "cuckoo-runtime:$VERSION" | gzip | remote 'gunzip | docker load'

echo "==> shipping files"
remote "mkdir -p '$REMOTE_DIR'"
scp -q deploy/runtime-compose.yml deploy/runtime-Caddyfile "$HOST:$REMOTE_DIR/"

echo "==> starting"
set_tag "$VERSION"
remote "cd '$REMOTE_DIR' && docker compose -f runtime-compose.yml up -d --remove-orphans"

# The first deploy waits for Caddy to fetch a certificate, which is why this
# is patient: a new name can take a minute to be answered for.
echo "==> checking"
for _ in $(seq 1 30); do
  if out=$(curl -fsS "https://$RUNTIME_DOMAIN/healthz" 2>/dev/null); then
    echo "$out"; echo "deployed $VERSION"; exit 0
  fi
  sleep 3
done
echo "the runtime did not answer on https://$RUNTIME_DOMAIN/healthz; see: ssh $HOST 'cd $REMOTE_DIR && docker compose -f runtime-compose.yml logs --tail 100'" >&2
exit 1
