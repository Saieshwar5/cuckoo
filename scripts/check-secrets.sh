#!/usr/bin/env bash
#
# check-secrets.sh — refuse to commit credentials.
#
# This repository is public. A pushed credential is scraped within minutes, and
# deleting the commit afterwards does not un-leak it. This runs before every
# commit for that reason alone.
#
# It matches only high-confidence patterns. A scanner that cries wolf gets
# switched off, and a switched-off scanner is worse than none — so there is
# deliberately no generic `password=` rule, because this repository legitimately
# contains postgres://cuckoo:cuckoo@localhost URLs in several places.
#
# Usage:
#   scripts/check-secrets.sh            # staged changes only (the hook)
#   scripts/check-secrets.sh --all      # every tracked file
#
# Escape hatch: put `ci:allow-secret` in a comment on the same line.
#
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODE="staged"
[[ "${1:-}" == "--all" ]] && MODE="all"

# Token shapes that are unambiguous: if one of these appears, it is a real
# credential or a deliberately realistic fake.
PATTERNS=(
  '-----BEGIN [A-Z ]*PRIVATE KEY-----'
  'gh[pousr]_[A-Za-z0-9]{36}'
  'github_pat_[A-Za-z0-9_]{60,}'
  'AKIA[0-9A-Z]{16}'
  'AIza[0-9A-Za-z_-]{35}'
  'xox[baprs]-[0-9A-Za-z-]{10,}'
  'sk-[A-Za-z0-9]{32,}'
  'sk_live_[A-Za-z0-9]{20,}'
  'rk_live_[A-Za-z0-9]{20,}'
)

# Files that must never be committed, whatever is inside them.
FORBIDDEN_NAMES='(^|/)(\.env(\.[a-z]+)?|id_rsa|id_ed25519|credentials\.json|google-services\.json|GoogleService-Info\.plist)$|\.(pem|key|p12|pfx|jks|keystore|mobileprovision)$'
ALLOWED_NAMES='(\.env\.example|\.pub)$'

# This script quotes the patterns above, so scanning it finds them.
SELF='scripts/check-secrets.sh'

if [[ "$MODE" == "staged" ]]; then
  files=$(git diff --cached --name-only --diff-filter=ACM)
else
  files=$(git ls-files)
fi

problems=0

# 1. Files that should never be tracked at all.
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ "$file" =~ $ALLOWED_NAMES ]] && continue
  if [[ "$file" =~ $FORBIDDEN_NAMES ]]; then
    echo "  FORBIDDEN FILE  $file"
    problems=$((problems + 1))
  fi
done <<< "$files"

# 2. Credential-shaped strings in the content.
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  [[ "$file" == "$SELF" ]] && continue
  [[ -f "$file" ]] || continue

  for pattern in "${PATTERNS[@]}"; do
    while IFS=: read -r lineno text; do
      [[ -z "$lineno" ]] && continue
      # Honour an explicit, reviewed exception.
      [[ "$text" == *"ci:allow-secret"* ]] && continue
      echo "  POSSIBLE SECRET $file:$lineno"
      echo "                  matched: ${pattern:0:40}"
      problems=$((problems + 1))
    done < <(grep -nE "$pattern" "$file" 2>/dev/null | head -5)
  done
done <<< "$files"

if [[ $problems -gt 0 ]]; then
  echo
  echo "check-secrets: FAIL — $problems finding(s)"
  echo
  echo "Nothing above should reach a public repository. Remove the value, put it"
  echo "in an environment variable, and rotate the credential — assume anything"
  echo "already committed is compromised."
  echo
  echo "If a match is genuinely not a secret, add 'ci:allow-secret' in a comment"
  echo "on that line."
  exit 1
fi

echo "check-secrets: OK — no credentials in ${MODE} files"
