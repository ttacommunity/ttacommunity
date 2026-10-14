#!/usr/bin/env bash
# Fail when a Go file, as staged, is not gofmt-clean. It reads git's copy, so a Windows
# checkout's CRLF line endings do not count. Usage: checks/gofmt.sh [<base>]
#   no argument: the staged Go files (the pre-commit hook); <base>: the Go files changed since it
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 1
if [ $# -gt 0 ]; then
  mapfile -d '' files < <(git diff --cached --name-only -z --diff-filter=ACM "$1" -- '*.go')
else
  mapfile -d '' files < <(git diff --cached --name-only -z --diff-filter=ACM -- '*.go')
fi
[ ${#files[@]} -eq 0 ] && exit 0
if ! command -v gofmt >/dev/null 2>&1; then
  printf '%s\n' 'REFUSED: Go files changed and gofmt is not on PATH (PowerShell: . scripts\env.ps1 first)' >&2
  exit 1
fi

unformatted=()
for f in "${files[@]}"; do
  [ -n "$(git show ":$f" | gofmt -l)" ] && unformatted+=("$f")
done
if [ ${#unformatted[@]} -gt 0 ]; then
  printf 'REFUSED: not gofmt-clean (gofmt -w <file>):\n' >&2
  printf '  %s\n' "${unformatted[@]}" >&2
  exit 1
fi
