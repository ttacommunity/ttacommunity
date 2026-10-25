#!/usr/bin/env bash
# Tests checks/engine-golden.sh in throwaway git repos: a base golden.json, then one change per case,
# which must pass or fail naming the version whose hash changed.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
check=$PWD/checks/engine-golden.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# commit writes golden.json in repo $1 as $2 (or removes it for -) and commits it.
commit() {
  mkdir -p "$1/engine/tests"
  if [ "$2" = - ]; then rm -f "$1/engine/tests/golden.json"; else echo "$2" >"$1/engine/tests/golden.json"; fi
  echo "$RANDOM" >"$1/touch"
  git -C "$1" add -A
  git -C "$1" -c user.name=test -c user.email=test@example.invalid commit -q -m change
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

failed=0
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
    echo "  $name: exit = $got, want $want; output: $out" >&2
    failed=1
  elif [ "$version" != - ] && ! grep -q "$version" <<<"$out"; then
    echo "  $name: output does not name $version; output: $out" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "engine-golden.test: failed" >&2
  exit 1
fi
echo "engine-golden.test: every case as expected"
