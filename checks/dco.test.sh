#!/usr/bin/env bash
# Tests checks/dco.sh in throwaway git repos: a base commit, then one pull request's commits per case,
# checked as authored by someone with the given author association.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start dco.test
check=$root/checks/dco.sh

author=(-c user.name=Outsider -c user.email=outsider@example.invalid)

# commit makes an empty commit by the outsider whose message ends as $1 says.
commit() {
  local msg
  case $1 in
    signed) msg=$'Change\n\nSigned-off-by: Outsider <outsider@example.invalid>' ;;
    none) msg='Change' ;;
    other) msg=$'Change\n\nSigned-off-by: Someone Else <else@example.invalid>' ;;
    upper) msg=$'Change\n\nSigned-off-by: Outsider <Outsider@Example.Invalid>' ;;
    body) msg=$'Change\n\nSigned-off-by: Outsider <outsider@example.invalid>\n\nA paragraph after it, so it is no trailer.' ;;
  esac
  git "${author[@]}" commit -q --allow-empty -m "$msg"
}

# repo creates a repo with a base commit then the commits $2 (comma-separated kinds), and prints its path.
repo() {
  local dir=$work/$1 kind trunk
  mkdir -p "$dir/checks"
  cp "$check" "$dir/checks/"
  (
    cd "$dir" || exit 1
    git init -q
    git "${author[@]}" commit -q --allow-empty -m base
    git tag base
    trunk=$(git symbolic-ref --short HEAD)
    IFS=',' read -ra kinds <<<"$2"
    for kind in "${kinds[@]}"; do
      if [ "$kind" = merge ]; then
        git checkout -q -b side && commit signed && git checkout -q "$trunk"
        git "${author[@]}" merge -q --no-ff -m 'Merge side' side
      else
        commit "$kind"
      fi
    done
  ) || return 1
  echo "$dir"
}

# case: name, the author association, the commits after base, the exit wanted.
cases=(
  "member-unsigned|MEMBER|none|0"
  "owner-unsigned|OWNER|none|0"
  "collaborator-unsigned|COLLABORATOR|none|0"
  "outsider-signed|CONTRIBUTOR|signed|0"
  "outsider-signed-other-case|FIRST_TIME_CONTRIBUTOR|upper|0"
  "outsider-merge-skipped|NONE|signed,merge|0"
  "outsider-unsigned|CONTRIBUTOR|none|1"
  "outsider-signed-by-someone-else|FIRST_TIMER|other|1"
  "outsider-sign-off-not-a-trailer|NONE|body|1"
  "outsider-second-of-two-unsigned|CONTRIBUTOR|signed,none|1"
  "no-association|||2"
  "no-commits|CONTRIBUTOR||2"
)

for c in "${cases[@]}"; do
  IFS='|' read -r name association commits want <<<"$c"
  if ! dir=$(repo "$name" "$commits"); then
    fail "$name: could not build the repo"
    continue
  fi
  out=$(cd "$dir" && bash checks/dco.sh base HEAD "$association" 2>&1)
  got=$?
  if [ "$got" -ne "$want" ]; then
    fail "$name: exit = $got, want $want; output: $out"
  fi
done

testlib_finish
