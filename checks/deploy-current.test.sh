#!/usr/bin/env bash
# Tests checks/deploy-current.sh in throwaway git repos: images pinned at one commit, then one change
# per case, which must leave the pins current or make the named image out of step.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
check=$PWD/checks/deploy-current.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# repo creates a repo whose deploy/ pins every image at its first commit, and prints its path.
repo() {
  local dir=$work/$1
  mkdir -p "$dir/checks" "$dir/deploy" "$dir/backend" "$dir/engine" "$dir/scripts"
  cp "$check" "$dir/checks/"
  printf 'go = "1.0.0"\nko = "1.0.0"\ncrane = "1.0.0"\nuv = "1.0.0"\nyq = "1.0.0"\n' >"$dir/mise.toml"
  echo 'package main' >"$dir/backend/main.go"
  echo 'package main' >"$dir/backend/main_test.go"
  echo 'x = 1' >"$dir/engine/engine.py"
  echo 'echo build' >"$dir/scripts/engine-image.sh"
  git -C "$dir" init -q
  git -C "$dir" add -A
  git -C "$dir" -c user.name=test -c user.email=test@example.invalid commit -q -m base
  local sha
  sha=$(git -C "$dir" rev-parse --short=8 HEAD)
  cat >"$dir/deploy/engine.yaml" <<EOF
image: ghcr.io/ttacommunity/ttacommunity/engine:sha-$sha
image: ghcr.io/ttacommunity/ttacommunity/api:sha-$sha
image: ghcr.io/ttacommunity/ttacommunity/load:sha-$sha
EOF
  git -C "$dir" add -A
  git -C "$dir" -c user.name=test -c user.email=test@example.invalid commit -q -m pin
  echo "$dir"
}

# case: name, the sed run on a file, that file, the exit wanted, the image the output must name (or -).
cases=(
  "nothing-changed|s/^//|mise.toml|0|-"
  "a-tool-no-image-uses|s/^yq = .*/yq = \"2.0.0\"/|mise.toml|0|-"
  "go|s/^go = .*/go = \"2.0.0\"/|mise.toml|1|api"
  "uv|s/^uv = .*/uv = \"2.0.0\"/|mise.toml|1|engine"
  "crane|s/^crane = .*/crane = \"2.0.0\"/|mise.toml|1|api"
  "backend-code|s/main/other/|backend/main.go|1|api"
  "backend-code-load|s/main/other/|backend/main.go|1|load"
  "a-go-test-no-binary-has|s/main/other/|backend/main_test.go|0|-"
  "go-load|s/^go = .*/go = \"2.0.0\"/|mise.toml|1|load"
  "engine-code|s/1/2/|engine/engine.py|1|engine"
)

failed=0
for c in "${cases[@]}"; do
  IFS='|' read -r name expr file want image <<<"$c"
  dir=$(repo "$name")
  sed -i "$expr" "$dir/$file"
  git -C "$dir" -c user.name=test -c user.email=test@example.invalid commit -q -am change --allow-empty
  out=$(bash "$dir/checks/deploy-current.sh" 2>&1)
  got=$?
  if [ "$got" -ne "$want" ]; then
    echo "  $name: exit = $got, want $want; output: $out" >&2
    failed=1
  elif [ "$image" != - ] && ! grep -q "ttacommunity/$image:sha-.*built from" <<<"$out"; then
    echo "  $name: output names no out-of-step $image image; output: $out" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "deploy-current.test: failed" >&2
  exit 1
fi
echo "deploy-current.test: every case as expected"
