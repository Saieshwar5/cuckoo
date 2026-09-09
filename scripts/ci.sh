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
if command -v python3 >/dev/null 2>&1; then
  step "python syntax"          bash -c 'python3 -m compileall -q sdk examples'
else
  skip "python syntax"          "python3 not installed"
fi
if [ -d apps/mobile/node_modules ]; then
  step "mobile typecheck"       bash -c 'cd apps/mobile && npx tsc --noEmit'
  step "mobile lint"            bash -c 'cd apps/mobile && npx eslint .'
  step "mobile format"          bash -c 'cd apps/mobile && npx prettier --check "**/*.{ts,tsx,json,md}" >/dev/null'
  step "mobile tests"           bash -c 'cd apps/mobile && npx jest --ci --silent >/dev/null 2>&1'
else
  skip "mobile checks"          "apps/mobile/node_modules missing, run make mobile-install"
fi
if [ -d sdk/typescript/node_modules ]; then
  step "ts sdk typecheck"       bash -c 'cd sdk/typescript && npx tsc --noEmit'
  step "ts sdk tests"           bash -c 'cd sdk/typescript && npm test --silent'
else
  skip "ts sdk checks"          "sdk/typescript/node_modules missing, run make ts-sdk-install"
fi
if [ -d examples/typescript/node_modules ]; then
  step "ts examples typecheck"  bash -c 'cd examples/typescript && npx tsc --noEmit'
else
  skip "ts examples"            "examples/typescript/node_modules missing"
fi
if [ -d runtime/node_modules ]; then
  step "runtime typecheck"      bash -c 'cd runtime && npx tsc --noEmit'
  step "runtime tests"          bash -c 'cd runtime && npm test --silent'
else
  skip "runtime checks"         "runtime/node_modules missing, run make runtime-install"
fi
if [ -d web/node_modules ]; then
  step "site typecheck"         bash -c 'cd web && npx astro check'
  step "site build"             bash -c 'cd web && npx astro build'
else
  skip "site checks"            "web/node_modules missing, run make site-install"
fi

# ── Generated code matches its source ────────────────────────────────────────
# Regenerate and compare the output with itself from a moment ago. Comparing
# against git instead would call correct-but-unstaged output stale, which is
# exactly the state a developer is in between `make gen` and `git add`.
gen_fingerprint() {
  find server/internal/store/gen -type f -name '*.go' -exec sha256sum {} + | sort | sha256sum
}
if [[ -x "$SQLC" ]]; then
  step "generated code is current" bash -c "
    before=\$($(declare -f gen_fingerprint); gen_fingerprint)
    (cd server && '$SQLC' generate) || exit 1
    after=\$($(declare -f gen_fingerprint); gen_fingerprint)
    if [[ \"\$before\" != \"\$after\" ]]; then
      echo 'internal/store/gen was stale — it has been regenerated; review and commit it'
      exit 1
    fi"
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
