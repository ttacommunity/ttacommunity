#!/usr/bin/env bash
# Fail when deploy/ pins one of our images built from older code than HEAD's.
# Each pin's tag names the commit it was built from (sha-<commit>); the code
# that image is built from must be the same at that commit and at HEAD.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

registry='registry.gitlab.com/unstablehomelab/ttacommunity'

# sources lists the paths image $1 is built from.
sources() {
  case $1 in
    api) echo "backend mise.toml" ;;
    engine) echo "engine scripts/engine-image.sh mise.toml" ;;
    *) return 1 ;;
  esac
}

mapfile -t pins < <(grep -rhoE "$registry/[a-z-]+:sha-[0-9a-f]+" deploy | sort -u)
if [ ${#pins[@]} -eq 0 ]; then
  echo "deploy-current: found no pinned image of ours in deploy/" >&2
  exit 1
fi

failed=0
for pin in "${pins[@]}"; do
  name=${pin#"$registry"/}; name=${name%%:*}
  commit=${pin##*:sha-}
  if ! paths=$(sources "$name"); then
    echo "  $pin: no sources known for image '$name'; add it to checks/deploy-current.sh" >&2
    failed=1; continue
  fi
  if ! git cat-file -e "$commit^{commit}" 2>/dev/null && [ "$(git rev-parse --is-shallow-repository)" = true ]; then
    git fetch --quiet --unshallow 2>/dev/null || true
  fi
  if ! git cat-file -e "$commit^{commit}" 2>/dev/null; then
    echo "  $pin: commit $commit is not in this repository" >&2
    failed=1; continue
  fi
  # shellcheck disable=SC2086 # paths is a list of paths
  if ! git diff --quiet "$commit" HEAD -- $paths; then
    echo "  $pin: built from $commit, but $paths changed since. Rebuild the image and pin it" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "deploy-current: deploy/ pins an image older than its code" >&2
  exit 1
fi
echo "deploy-current: ${#pins[@]} pinned image(s) built from HEAD's code"
