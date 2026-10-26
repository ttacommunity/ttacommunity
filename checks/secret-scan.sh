#!/usr/bin/env bash
# Refuse content that looks like a credential, before it leaves this machine.
# Used by the git hooks and scripts/ci.sh. The broad entropy sweep is CI's
# gitleaks step; this one keeps to formats that are never innocent.
#
# Usage: checks/secret-scan.sh <file>...               scan those files on disk
#        checks/secret-scan.sh --staged                scan what is staged, as staged
#        checks/secret-scan.sh --rev <commit> <file>...  scan those files as that commit has them
#        checks/secret-scan.sh --all                   scan every tracked and unignored file
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

files=()
spec=""
case "${1:-}" in
  --staged)
    spec=:
    mapfile -d '' files < <(git diff --cached --name-only -z --diff-filter=ACM) ;;
  --rev)
    if ! git rev-parse --verify --quiet "${2:-}^{commit}" >/dev/null; then
      printf 'REFUSED: --rev needs a commit, and %s is not one\n' "${2:-nothing}" >&2
      exit 1
    fi
    spec="$2:"
    files=("${@:3}") ;;
  --all)
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      echo "REFUSED: --all lists files with git, and this is not a git checkout" >&2
      exit 1
    fi
    mapfile -d '' files < <(git ls-files -co --exclude-standard -z)
    if [ ${#files[@]} -eq 0 ]; then
      echo "REFUSED: --all found no files to scan" >&2
      exit 1
    fi ;;
  *) files=("$@") ;;
esac
[ ${#files[@]} -eq 0 ] && exit 0

copy=$(mktemp)
trap 'rm -f "$copy"' EXIT

# fetch writes the file's content to $copy: from disk, or from git when $spec names the index or a commit.
fetch() {
  if [ -z "$spec" ]; then
    [ -f "$1" ] && cp -- "$1" "$copy"
  else
    git cat-file -e "$spec$1" 2>/dev/null && git show "$spec$1" >"$copy"
  fi
}

patterns=(
  'glpat-[A-Za-z0-9_-]{20,}'                               # GitLab access token
  'gldt-[A-Za-z0-9_-]{20,}'                                # GitLab deploy token
  'glrt-[A-Za-z0-9_-]{20,}'                                # GitLab runner token
  'gh[pousr]_[A-Za-z0-9]{36}'                              # GitHub tokens (personal, OAuth, app, refresh)
  'github_pat_[A-Za-z0-9_]{60,}'                           # GitHub fine-grained token
  'AKIA[0-9A-Z]{16}'                                       # AWS access key id
  '-----BEGIN [A-Z ]*PRIVATE KEY-----'                     # any private key
  'xox[baprs]-[A-Za-z0-9-]{10,}'                           # Slack
  'cf(k|ut|at|ast)_[A-Za-z0-9]{40,}'                       # Cloudflare key, user, account and Access service tokens
  'AGE-SECRET-KEY-(PQ-)?1[QPZRY9X8GF2TVDW0S3JN54KHCE6MUA7L]{58}' # age identity
  'GOCSPX-[A-Za-z0-9_-]{20,}'                              # Google OAuth client secret (shape from scanners, no provider spec)
  '(^|[^A-Za-z0-9_])re_[A-Za-z0-9]{8,}_[A-Za-z0-9]{16,}'   # Resend API key (shape from scanners, no provider spec)
  '[MNO][A-Za-z0-9_-]{23,27}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}' # Discord bot token
)

problems=0
for f in "${files[@]}"; do
  [ "$f" = checks/secret-scan.sh ] && continue
  : >"$copy"
  fetch "$f" || continue
  grep -Iq . "$copy" 2>/dev/null || continue
  for p in "${patterns[@]}"; do
    if line=$(grep -nEo -e "$p" "$copy" | head -1) && [ -n "$line" ]; then
      printf '  %s:%s looks like a credential (%s)\n' "$f" "${line%%:*}" "$p" >&2
      problems=$((problems + 1))
    fi
  done
  while IFS=: read -r n url; do
    pw=${url#*://*:}
    case ${pw%@} in
      local-only*|ci-only-*|password|*'$'*|*%*|*'<'*|*'{'*) ;;
      *) printf '  %s:%s a Postgres URL with a password (local-only… and ci-only-… are the safe ones)\n' "$f" "$n" >&2
         problems=$((problems + 1)) ;;
    esac
  done < <(grep -nEo "postgres(ql)?://[^:/@[:space:]]+:[^@/[:space:]\"']+@" "$copy")
  case $f in
    *.yaml|*.yml)
      if grep -qE '^kind:[[:space:]]*Secret' "$copy" && grep -qE '^[[:space:]]*(data|stringData):' "$copy"; then
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
