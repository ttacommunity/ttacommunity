#!/usr/bin/env bash
# Tests checks/deploy-current.sh in throwaway git repos: images pinned at one commit, then one change
# per case, which must leave the pins current or make the named image out of step.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start deploy-current.test
check=$root/checks/deploy-current.sh

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
  commit_in "$dir" -m base
  local sha
  sha=$(git -C "$dir" rev-parse --short=8 HEAD)
  cat >"$dir/deploy/engine.yaml" <<EOF
image: ghcr.io/ttacommunity/ttacommunity/engine:sha-$sha
image: ghcr.io/ttacommunity/ttacommunity/api:sha-$sha
image: ghcr.io/ttacommunity/ttacommunity/load:sha-$sha
EOF
  git -C "$dir" add -A
  commit_in "$dir" -m pin
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

for c in "${cases[@]}"; do
  IFS='|' read -r name expr file want image <<<"$c"
  dir=$(repo "$name")
  sed -i "$expr" "$dir/$file"
  commit_in "$dir" -am change --allow-empty
  out=$(bash "$dir/checks/deploy-current.sh" 2>&1)
  got=$?
  if [ "$got" -ne "$want" ]; then
    fail "$name: exit = $got, want $want; output: $out"
  elif [ "$image" != - ] && ! grep -q "ttacommunity/$image:sha-.*built from" <<<"$out"; then
    fail "$name: output names no out-of-step $image image; output: $out"
  fi
done

# pin case: name, a file added to the current repo (- for none), the image:tag it pins (SHA is the pins' commit),
# the directory checked (- for the default), the exit wanted, text the output must hold (- for any).
pin_cases=(
  "dev-current|deploy/dev/api.yaml|api:sha-SHA|deploy/dev|0|-"
  "dev-bad-pin|deploy/dev/api.yaml|api:sha-0000000a|deploy/dev|1|commit 0000000a is not in this repository"
  "production-leaves-out-dev|deploy/dev/api.yaml|api:sha-0000000a|-|0|-"
  "dev-with-no-pins|-|-|deploy/dev|1|found no pinned image"
  "a-commit-not-here|deploy/more.yaml|api:sha-0000000a|-|1|commit 0000000a is not in this repository"
  "an-unknown-image|deploy/more.yaml|worker:sha-SHA|-|1|no sources known for image 'worker'"
  "an-unknown-directory|-|-|deploy/other|64|checks deploy or deploy/dev"
)

for c in "${pin_cases[@]}"; do
  IFS='|' read -r name file pin target want text <<<"$c"
  dir=$(repo "$name")
  if [ "$file" != - ]; then
    sha=$(git -C "$dir" rev-parse --short=8 HEAD~1)
    mkdir -p "$(dirname "$dir/$file")"
    echo "image: ghcr.io/ttacommunity/ttacommunity/${pin//SHA/$sha}" >"$dir/$file"
    git -C "$dir" add -A
    commit_in "$dir" -m more
  fi
  args=()
  [ "$target" = - ] || args=("$target")
  out=$(bash "$dir/checks/deploy-current.sh" "${args[@]}" 2>&1)
  got=$?
  if [ "$got" -ne "$want" ]; then
    fail "$name: exit = $got, want $want; output: $out"
  elif [ "$text" != - ] && ! grep -qF "$text" <<<"$out"; then
    fail "$name: output lacks '$text'; output: $out"
  fi
done

testlib_finish
