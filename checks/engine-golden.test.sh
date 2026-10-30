#!/usr/bin/env bash
# Tests checks/engine-golden.sh in throwaway git repos: a base golden.json, then one change per case,
# which must pass or fail naming the version whose hash changed.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start engine-golden.test
check=$root/checks/engine-golden.sh

# commit writes golden.json in repo $1 as $2 (or removes it for -) and commits it.
commit() {
  mkdir -p "$1/engine/tests"
  if [ "$2" = - ]; then rm -f "$1/engine/tests/golden.json"; else echo "$2" >"$1/engine/tests/golden.json"; fi
  echo "$RANDOM" >"$1/touch"
  git -C "$1" add -A
  commit_in "$1" -m change
}

# case: name, the base's golden.json (- for none), the head's, the exit wanted, the version the output must name.
cases=(
  'no-golden-on-the-base|-|{"1.0.0": "aaa"}|0|-'
  'unchanged|{"1.0.0": "aaa"}|{"1.0.0": "aaa"}|0|-'
  'a-new-version|{"1.0.0": "aaa"}|{"1.0.0": "aaa", "1.1.0": "bbb"}|0|-'
  'an-old-hash-changed|{"1.0.0": "aaa"}|{"1.0.0": "bbb"}|1|1.0.0'
  'an-old-version-removed|{"1.0.0": "aaa"}|{"1.1.0": "bbb"}|1|1.0.0'
  'two-hashes-unchanged|{"1.0.0": {"ratings": "aaa", "derived": "bbb"}}|{"1.0.0": {"ratings": "aaa", "derived": "bbb"}}|0|-'
  'a-derived-hash-changed|{"1.0.0": {"ratings": "aaa", "derived": "bbb"}}|{"1.0.0": {"ratings": "aaa", "derived": "ccc"}}|1|1.0.0'
)

for c in "${cases[@]}"; do
  IFS='|' read -r name base head want version <<<"$c"
  dir=$work/$name
  mkdir -p "$dir/checks"
  cp "$check" "$dir/checks/"
  git -C "$dir" init -q
  commit "$dir" "$base"
  base_sha=$(git -C "$dir" rev-parse HEAD)
  commit "$dir" "$head"
  out=$(bash "$dir/checks/engine-golden.sh" "$base_sha" 2>&1)
  got=$?
  if [ "$got" -ne "$want" ]; then
    fail "$name: exit = $got, want $want; output: $out"
  elif [ "$version" != - ] && ! grep -q "$version" <<<"$out"; then
    fail "$name: output does not name $version; output: $out"
  fi
done

testlib_finish
