#!/usr/bin/env bash
# Fail when deploy/ pins one of our images built from older code than HEAD's.
# Each pin's tag names the commit it was built from (sha-<commit>); the code
# that image is built from must be the same at that commit and at HEAD.
# Usage: checks/deploy-current.sh [deploy | deploy/dev]   (branch dev's own pins)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

dir=${1:-deploy}
case $dir in
  deploy) exclude=(--exclude-dir=dev) ;;
  deploy/dev) exclude=() ;;
  *) echo "deploy-current: checks deploy or deploy/dev, not '$dir'" >&2; exit 64 ;;
esac

registry='ghcr.io/ttacommunity/ttacommunity'

# sources lists the paths image $1 is built from.
sources() {
  case $1 in
    api | load) echo "backend" ;;
    engine) echo "engine scripts/engine-image.sh" ;;
    *) return 1 ;;
  esac
}

# tools lists the mise.toml tools image $1's build installs (.github/workflows/images.yml).
tools() {
  case $1 in
    api | load) echo "go ko crane" ;;
    engine) echo "uv crane" ;;
  esac
}

# tool_pins prints mise.toml's lines for the tools $2 at commit $1.
tool_pins() {
  git show "$1:mise.toml" | grep -E "^(${2// /|}) = "
}

mapfile -t pins < <(grep -rhoE "${exclude[@]}" "$registry/[a-z-]+:sha-[0-9a-f]+" "$dir" | sort -u)
if [ ${#pins[@]} -eq 0 ]; then
  echo "deploy-current: found no pinned image of ours in $dir/" >&2
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
  elif [ "$(tool_pins "$commit" "$(tools "$name")")" != "$(tool_pins HEAD "$(tools "$name")")" ]; then
    echo "  $pin: built from $commit, but its tools ($(tools "$name")) changed in mise.toml since. Rebuild the image and pin it" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "deploy-current: $dir/ is out of step with its code" >&2
  exit 1
fi
echo "deploy-current: ${#pins[@]} pinned image(s) built from HEAD's code"
