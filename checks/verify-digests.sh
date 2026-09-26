#!/usr/bin/env bash
# Ask each registry whether every pinned image digest in the repo exists.
# A fabricated digest looks exactly like a real one; only the registry knows.
# Private GitLab images use CI_REGISTRY_USER/CI_JOB_TOKEN when they are set.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

command -v curl >/dev/null 2>&1 || { echo "verify-digests: curl is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "verify-digests: jq is required" >&2; exit 1; }

ACCEPT='application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json'
ACCEPT="$ACCEPT,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.v2+json"

# status prints the HTTP status a registry gives for image reference $1.
status() {
  local ref=$1 name digest registry repo host url headers realm service token
  local auth=()
  name=${ref%@*}; digest=${ref#*@}; name=${name%:*}
  case ${name%%/*} in
    *.*|*:*|localhost) registry=${name%%/*}; repo=${name#*/} ;;
    *) registry=docker.io; repo=$name; [[ $repo == */* ]] || repo="library/$repo" ;;
  esac
  host=$registry; [ "$registry" = docker.io ] && host="registry-1.docker.io"
  url="https://$host/v2/$repo/manifests/$digest"

  headers=$(curl -sSI --max-time 25 -H "Accept: $ACCEPT" "$url")
  if printf '%s' "$headers" | grep -qi '^HTTP/[0-9.]* 401'; then
    realm=$(printf '%s' "$headers" | tr -d '\r' | sed -n 's/^[Ww]ww-[Aa]uthenticate:.*realm="\([^"]*\)".*/\1/p')
    service=$(printf '%s' "$headers" | tr -d '\r' | sed -n 's/^[Ww]ww-[Aa]uthenticate:.*service="\([^"]*\)".*/\1/p')
    if [ "$registry" = registry.gitlab.com ] && [ -n "${CI_JOB_TOKEN:-}" ]; then
      auth=(-u "${CI_REGISTRY_USER}:${CI_JOB_TOKEN}")
    fi
    token=$(curl -sS --max-time 25 "${auth[@]}" "$realm?scope=repository:$repo:pull&service=$service" |
      jq -r '.token // .access_token // empty')
    [ -n "$token" ] && headers=$(curl -sSI --max-time 25 -H "Accept: $ACCEPT" -H "Authorization: Bearer $token" "$url")
  fi
  printf '%s' "$headers" | head -1 | tr -d '\r' | awk '{print $2}'
}

mapfile -t sources < <(git ls-files -co --exclude-standard |
  grep -E '(^|/)(Dockerfile|docker-compose\.yml|\.ko\.yaml|\.gitlab-ci\.yml)$|^deploy/|^scripts/')
refs=$(grep -hoE '[a-z0-9][a-z0-9./_-]*(:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}' "${sources[@]}" | sort -u)
if [ -z "$refs" ]; then
  echo "verify-digests: found no pinned references, which is itself suspicious" >&2
  exit 1
fi

problems=0; checked=0
while IFS= read -r ref; do
  checked=$((checked + 1))
  code=$(status "$ref")
  case $code in
    200) printf '  ok       %s\n' "$ref" ;;
    404) printf '  MISSING  %s\n           if any part was typed rather than resolved, it is invented\n' "$ref"
         problems=$((problems + 1)) ;;
    *)   printf '  ?        %s\n           registry said "%s"; NOT treated as a pass\n' "$ref" "$code"
         problems=$((problems + 1)) ;;
  esac
done <<<"$refs"

if [ "$problems" -gt 0 ]; then
  echo "verify-digests: $problems of $checked reference(s) did not resolve" >&2
  exit 1
fi
echo "  verify-digests: all $checked reference(s) resolve"
