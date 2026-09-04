#!/usr/bin/env bash
#
# ci.sh — the full verification run. What a CI server would have done.
#
# Run this before opening a pull request. It differs from `make test` in two
# ways that matter:
#
#   * It recreates the test database and migrates from empty. Your development
#     database already has every table, so a broken migration — wrong order, a
#     dependency that only exists because you added it by hand — keeps working
#     locally forever and fails on a self-hoster's first install. Migrating from
#     nothing is the only thing that catches it.
#
#   * It checks that the generated database code still matches the SQL it came
#     from. Edit a query, forget `make gen`, and your Go and your schema
#     disagree silently. No test catches that.
#
# Usage:
#   scripts/ci.sh              # everything
#   SKIP_LINT=1 scripts/ci.sh  # without golangci-lint
#
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TEST_DB_NAME="cuckoo_test"
TEST_DATABASE_URL="${CUCKOO_TEST_DATABASE_URL:-postgres://cuckoo:cuckoo@localhost:5433/${TEST_DB_NAME}?sslmode=disable}"
SQLC="$ROOT/bin/sqlc"

started=$(date +%s)
failed=0
declare -a results

step() {
  local name="$1"; shift
  local output status
  printf '  %-34s' "$name"
  output=$("$@" 2>&1); status=$?
  if [[ $status -eq 0 ]]; then
    printf '\033[32mok\033[0m\n'
    results+=("ok    $name")
  else
    printf '\033[31mFAIL\033[0m\n'
    printf '%s\n' "$output" | tail -40 | sed 's/^/      /'
    results+=("FAIL  $name")
    failed=1
  fi
}

skip() {
  printf '  %-34s\033[33mskipped\033[0m — %s\n' "$1" "$2"
  results+=("skip  $1 ($2)")
}

echo "cuckoo ci"
echo

# ── Static checks ────────────────────────────────────────────────────────────
step "file size limit"        ./scripts/check-file-size.sh
step "no credentials"         ./scripts/check-secrets.sh --all
step "gofmt"                  bash -c 'cd server && test -z "$(gofmt -l .)" || { gofmt -l .; false; }'
step "go vet"                 bash -c 'cd server && go vet ./...'
step "build"                  bash -c 'cd server && go build ./...'

# ── Generated code matches its source ────────────────────────────────────────
if [[ -x "$SQLC" ]]; then
  step "generated code is current" bash -c "cd server && '$SQLC' generate && git diff --quiet -- internal/store/gen || { echo 'internal/store/gen is stale — run make gen and commit the result'; git --no-pager diff -- internal/store/gen; false; }"
else
  skip "generated code is current" "sqlc missing, run make tools"
fi

# ── Database from empty ──────────────────────────────────────────────────────
if docker compose exec -T postgres pg_isready -U cuckoo -q 2>/dev/null; then
  step "recreate test database" bash -c "
    docker compose exec -T postgres psql -U cuckoo -d postgres -q \
      -c 'DROP DATABASE IF EXISTS ${TEST_DB_NAME} WITH (FORCE)' \
      -c 'CREATE DATABASE ${TEST_DB_NAME} OWNER cuckoo'"

  # The suite migrates on first database contact, so this single test proves
  # the schema builds from nothing before the rest of the suite depends on it.
  step "migrate from empty"     bash -c "cd server && CUCKOO_TEST_DATABASE_URL='$TEST_DATABASE_URL' go test ./internal/users/ -run TestCreateAndGet -count=1"

  step "tests (-race)"          bash -c "cd server && CUCKOO_TEST_DATABASE_URL='$TEST_DATABASE_URL' go test ./... -race -count=1"
else
  skip "recreate test database" "postgres not running, run make up"
  skip "migrate from empty"     "postgres not running"
  skip "tests (-race)"          "postgres not running"
  failed=1
fi

# ── Lint ─────────────────────────────────────────────────────────────────────
if [[ -n "${SKIP_LINT:-}" ]]; then
  skip "golangci-lint" "SKIP_LINT set"
elif command -v golangci-lint >/dev/null 2>&1 || [[ -x "$ROOT/bin/golangci-lint" ]]; then
  step "golangci-lint" bash -c "cd server && PATH='$ROOT/bin:$PATH' golangci-lint run"
else
  skip "golangci-lint" "not installed, run make tools"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
elapsed=$(( $(date +%s) - started ))
echo
printf '  %s in %ds\n' "$( [[ $failed -eq 0 ]] && echo 'PASSED' || echo 'FAILED' )" "$elapsed"

skipped=$(printf '%s\n' "${results[@]}" | grep -c '^skip' || true)
if [[ $skipped -gt 0 ]]; then
  echo
  printf '  \033[33m%d check(s) did not run.\033[0m A run with skips is not a clean run.\n' "$skipped"
fi

exit $failed
