#!/usr/bin/env bash
#
# watch.sh — rerun the tests whenever a file changes.
#
# This is not a gate; it is the other half of what CI culture is actually for.
# The hooks tell you when something is broken. This tells you the moment it
# breaks, while the change is still in your head.
#
# Polls rather than using inotify so it needs no dependency and behaves the same
# everywhere.
#
# Usage:
#   scripts/watch.sh                     # whole suite, short mode (no database)
#   scripts/watch.sh ./internal/users    # one package
#   WATCH_FULL=1 scripts/watch.sh        # include database tests
#
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT/server"

TARGET="${1:-./...}"
INTERVAL="${WATCH_INTERVAL:-1}"
TEST_DATABASE_URL="${CUCKOO_TEST_DATABASE_URL:-postgres://cuckoo:cuckoo@localhost:5433/cuckoo_test?sslmode=disable}"

if [[ -n "${WATCH_FULL:-}" ]]; then
  ARGS=(-count=1)
  MODE="full (database included)"
else
  ARGS=(-short -count=1)
  MODE="short (no database)"
fi

fingerprint() {
  find . -type f \( -name '*.go' -o -name '*.sql' \) -not -path './tmp/*' \
    -printf '%T@ %p\n' 2>/dev/null | sort | cksum
}

run() {
  clear
  printf '\033[1mwatching\033[0m %s — %s — %s\n' "$TARGET" "$MODE" "$(date +%H:%M:%S)"
  echo
  if CUCKOO_TEST_DATABASE_URL="$TEST_DATABASE_URL" go test "$TARGET" "${ARGS[@]}" 2>&1; then
    printf '\n\033[32m  PASS\033[0m\n'
  else
    printf '\n\033[31m  FAIL\033[0m\n'
  fi
  printf '\n  Ctrl-C to stop.\n'
}

trap 'echo; echo "stopped."; exit 0' INT

run
last=$(fingerprint)

while true; do
  sleep "$INTERVAL"
  current=$(fingerprint)
  if [[ "$current" != "$last" ]]; then
    last="$current"
    run
  fi
done
