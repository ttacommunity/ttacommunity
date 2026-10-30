# shellcheck shell=bash
# Shared by the checks' tests (checks/*.test.sh): a throwaway directory, commits as a test user, and the result.
# Source it from a test, call testlib_start <name>, record failures with fail, and end with testlib_finish.

# testlib_start moves to the repository's root, sets $root, makes $work (removed on exit) and clears failures.
testlib_start() {
  test_name=$1
  cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1
  # shellcheck disable=SC2034 # root is for the test that sources this
  root=$PWD
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT
  failed=0
}

# fail records a failed case and prints $1.
fail() {
  echo "  $1" >&2
  failed=1
}

# commit_in commits in repository $1 as the test user, passing the rest to git commit.
commit_in() {
  local dir=$1
  shift
  git -C "$dir" -c user.name=test -c user.email=test@example.invalid commit -q "$@"
}

# testlib_finish prints the result, and exits 1 when a case failed.
testlib_finish() {
  if [ "$failed" -ne 0 ]; then
    echo "$test_name: failed" >&2
    exit 1
  fi
  echo "$test_name: every case as expected"
}
