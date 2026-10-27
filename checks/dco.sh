#!/usr/bin/env bash
# Fail when a pull request from outside the maintainers has a commit without a Signed-off-by trailer
# in its author's name (CONTRIBUTING.md). Exit 1 on such a commit, 2 when the input cannot be checked.
# Usage: checks/dco.sh <base> <head> <author association>   (GitHub's pull_request.author_association)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

if [ $# -ne 3 ]; then
  echo "usage: checks/dco.sh <base> <head> <author association>" >&2
  exit 64
fi
base=$1 head=$2 association=$3

case $association in
  OWNER | MEMBER | COLLABORATOR)
    echo "dco: the author is a maintainer ($association): no sign-off needed"
    exit 0
    ;;
  '')
    echo "dco: no author association given: run it with the pull request's" >&2
    exit 2
    ;;
esac

if ! commits=$(git rev-list --no-merges "$base..$head"); then
  echo "dco: cannot list the commits $base..$head" >&2
  exit 2
fi
if [ -z "$commits" ]; then
  echo "dco: no commits between $base and $head" >&2
  exit 2
fi

failed=0
for c in $commits; do
  email=$(git log -1 --format=%ae "$c")
  signers=$(git log -1 --format='%(trailers:key=Signed-off-by,valueonly)' "$c" | grep -oE '<[^>]+>' | tr -d '<>')
  if ! grep -qixF -- "$email" <<<"$signers"; then
    echo "  $(git log -1 --format='%h %s' "$c"): no Signed-off-by with its author's email" >&2
    failed=1
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "dco: sign off each commit (git rebase --signoff $base, then push again); see CONTRIBUTING.md" >&2
  exit 1
fi
echo "dco: every commit is signed off by its author ($association)"
