#!/usr/bin/env bash
# Fail when a rendered env value in deploy/ is not a string: YAML reads 123, true or a bare value: as
# another type, and Kubernetes refuses the whole apply. Exit 1 on such a value, 2 when rendering fails.
# Usage: checks/deploy-env-strings.sh [kustomization dir...]   (default: deploy deploy/dev)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2
[ $# -gt 0 ] || set -- deploy deploy/dev

# Every map with env (containers and initContainers at any depth), each value that is not a string.
# shellcheck disable=SC2016 # $kind, $res and $ctr are yq's variables, not the shell's
query='.kind as $kind | .metadata.name as $res | .. | select(tag == "!!map" and has("env")) | .name as $ctr
  | .env[] | select(has("value") and (.value | tag) != "!!str")
  | $kind + "/" + $res + " container " + $ctr + ": " + .name + " is " + (.value | tag) + ", quote it"'

failed=0
for dir in "$@"; do
  if ! rendered=$(kustomize build "$dir"); then
    echo "deploy-env-strings: kustomize could not render $dir" >&2
    exit 2
  fi
  if ! bad=$(yq -N "$query" <<<"$rendered"); then
    echo "deploy-env-strings: yq could not read $dir" >&2
    exit 2
  fi
  if [ -n "$bad" ]; then
    echo "  $dir: ${bad//$'\n'/$'\n'  $dir: }" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "deploy-env-strings: an env value is not a string" >&2
  exit 1
fi
echo "deploy-env-strings: every env value in $* is a string"
