#!/usr/bin/env bash
#
# check-file-size.sh — fail the build when a source file grows too long.
#
# A file over the limit is a design signal, not a style nit: it means a package
# is doing more than one job and should be split. Enforced in `make test` so it
# fails the same way for a human and for an agent.
#
# Usage:
#   scripts/check-file-size.sh              # check, exit 1 on any violation
#   scripts/check-file-size.sh --list       # print every checked file + count
#   scripts/check-file-size.sh --top 10     # print the 10 longest files
#   CUCKOO_FILE_LINE_LIMIT=400 scripts/check-file-size.sh
#
set -euo pipefail

LIMIT="${CUCKOO_FILE_LINE_LIMIT:-500}"
WARN_AT=$(( LIMIT * 80 / 100 ))   # warn (but pass) once a file is 80% of the way

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODE="check"
TOP_N=10
while [[ $# -gt 0 ]]; do
  case "$1" in
    --list) MODE="list"; shift ;;
    --top)  MODE="top"; TOP_N="${2:-10}"; shift 2 ;;
    --limit) LIMIT="$2"; WARN_AT=$(( LIMIT * 80 / 100 )); shift 2 ;;
    -h|--help) sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# Extensions we hold to the limit. Prose (.md) and lockfiles are exempt.
EXTENSIONS='go ts tsx js jsx sql sh py'

# Paths that are generated, vendored, or otherwise not hand-written.
is_excluded() {
  case "$1" in
    */store/gen/*|*/gen/*)      return 0 ;;  # sqlc output
    *_gen.go|*.pb.go|*_templ.go) return 0 ;;
    bin/*|dist/*|build/*|tmp/*) return 0 ;;
    */node_modules/*|vendor/*)  return 0 ;;
    .git/*)                     return 0 ;;
    # Safety net for the find fallback before `git init`: these are gitignored
    # working directories, not part of the project.
    proj-docs/*|ref-proj/*)     return 0 ;;
    *) return 1 ;;
  esac
}

# Prefer git so .gitignore is honoured for free; fall back to find before `git init`.
list_candidates() {
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git ls-files --cached --others --exclude-standard
  else
    find . -type f -not -path './.git/*' | sed 's|^\./||'
  fi
}

matches_extension() {
  local ext="${1##*.}"
  [[ "$1" == *.* ]] || return 1
  for e in $EXTENSIONS; do [[ "$ext" == "$e" ]] && return 0; done
  return 1
}

# Collect "lines<TAB>path" for every file in scope.
measured=""
while IFS= read -r file; do
  [[ -f "$file" ]] || continue
  matches_extension "$file" || continue
  is_excluded "$file" && continue
  lines=$(wc -l < "$file" | tr -d ' ')
  measured+="${lines}	${file}"$'\n'
done < <(list_candidates)

if [[ -z "$measured" ]]; then
  echo "check-file-size: no source files to check"
  exit 0
fi

sorted=$(printf '%s' "$measured" | sort -rn)

case "$MODE" in
  list) printf '%s\n' "$sorted" | awk -F'\t' '{printf "%6s  %s\n", $1, $2}'; exit 0 ;;
  top)  printf '%s\n' "$sorted" | head -n "$TOP_N" | awk -F'\t' '{printf "%6s  %s\n", $1, $2}'; exit 0 ;;
esac

violations=$(printf '%s\n' "$sorted" | awk -F'\t' -v lim="$LIMIT" '$1 > lim')
warnings=$(printf '%s\n' "$sorted" | awk -F'\t' -v lim="$LIMIT" -v warn="$WARN_AT" '$1 > warn && $1 <= lim')
total=$(printf '%s\n' "$sorted" | grep -c . || true)

if [[ -n "$warnings" ]]; then
  echo "check-file-size: approaching the ${LIMIT}-line limit —"
  printf '%s\n' "$warnings" | awk -F'\t' '{printf "  %4s lines  %s\n", $1, $2}'
fi

if [[ -n "$violations" ]]; then
  count=$(printf '%s\n' "$violations" | grep -c . || true)
  echo
  echo "check-file-size: FAIL — ${count} file(s) over the ${LIMIT}-line limit:"
  printf '%s\n' "$violations" | awk -F'\t' -v lim="$LIMIT" \
    '{printf "  %4s lines (%+d)  %s\n", $1, $1 - lim, $2}'
  echo
  echo "Split the file by responsibility. If the limit is genuinely wrong for this"
  echo "repo, change CUCKOO_FILE_LINE_LIMIT in the Makefile — do not special-case a file."
  exit 1
fi

echo "check-file-size: OK — ${total} files, all within ${LIMIT} lines"
