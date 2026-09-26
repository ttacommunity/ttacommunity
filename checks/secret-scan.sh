#!/usr/bin/env bash
# Refuse content that looks like a credential, before it leaves this machine.
# Used by the git hooks and scripts/ci.sh. The broad entropy sweep is CI's
# secret_detection job; this one keeps to formats that are never innocent.
#
# Usage: checks/secret-scan.sh <file>...   scan those files
#        checks/secret-scan.sh --staged    scan what is staged
#        checks/secret-scan.sh --all       scan every tracked and unignored file
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

files=()
case "${1:-}" in
  --staged) mapfile -d '' files < <(git diff --cached --name-only -z --diff-filter=ACM) ;;
  --all)    mapfile -d '' files < <(git ls-files -co --exclude-standard -z) ;;
  *)        files=("$@") ;;
esac
[ ${#files[@]} -eq 0 ] && exit 0

patterns=(
  'glpat-[A-Za-z0-9_-]{20,}'                               # GitLab access token
  'gldt-[A-Za-z0-9_-]{20,}'                                # GitLab deploy token
  'glrt-[A-Za-z0-9_-]{20,}'                                # GitLab runner token
  'ghp_[A-Za-z0-9]{36}'                                    # GitHub token
  'github_pat_[A-Za-z0-9_]{60,}'                           # GitHub fine-grained token
  'AKIA[0-9A-Z]{16}'                                       # AWS access key id
  '-----BEGIN [A-Z ]*PRIVATE KEY-----'                     # any private key
  'xox[baprs]-[A-Za-z0-9-]{10,}'                           # Slack
  '[MNO][A-Za-z0-9_-]{23,27}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}' # Discord bot token
)

problems=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  [ "$f" = checks/secret-scan.sh ] && continue
  grep -Iq . "$f" 2>/dev/null || continue
  for p in "${patterns[@]}"; do
    if line=$(grep -nEo -e "$p" "$f" | head -1) && [ -n "$line" ]; then
      printf '  %s:%s looks like a credential (%s)\n' "$f" "${line%%:*}" "$p" >&2
      problems=$((problems + 1))
    fi
  done
  case $f in
    *.yaml|*.yml)
      if grep -qE '^kind:[[:space:]]*Secret' "$f" && grep -qE '^[[:space:]]*(data|stringData):' "$f"; then
        printf '  %s: a Secret carrying data. Secrets are created by hand, never committed\n' "$f" >&2
        problems=$((problems + 1))
      fi ;;
  esac
done

if [ "$problems" -gt 0 ]; then
  echo "REFUSED: $problems match(es) above look like credentials. Once pushed, only rotation fixes a leak." >&2
  exit 1
fi
echo "  secret-scan: ${#files[@]} file(s), nothing found"
