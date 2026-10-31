#!/usr/bin/env bash
# Tests checks/secret-scan.sh and .githooks/pre-push in throwaway git repos. The fake credentials are
# built here at run time, so no credential-shaped string is ever committed.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start secret-scan.test

# rep prints the character $1 repeated $2 times.
rep() { printf "%${2}s" '' | tr ' ' "$1"; }

# repo creates a repo holding the scan and the pre-push hook, and prints its path.
repo() {
  local dir=$work/$1
  mkdir -p "$dir/checks" "$dir/.githooks"
  cp "$root/checks/secret-scan.sh" "$dir/checks/"
  cp "$root/.githooks/pre-push" "$dir/.githooks/"
  git -C "$dir" init -q
  echo "$dir"
}


pg=postgres://u
# case: name, the file's one line, the exit wanted.
cases=(
  "clean|nothing here|0"
  "github token|t=gh""p_$(rep a 36)|1"
  "cloudflare user token|t=cf""ut_$(rep a 48)|1"
  "cloudflare account token|t=cf""at_$(rep a 48)|1"
  "cloudflare global key|t=cf""k_$(rep a 48)|1"
  "cloudflare access service secret|t=cf""ast_$(rep a 48)|1"
  "age secret key|AGE-SECRET""-KEY-1$(rep Q 58)|1"
  "age post-quantum secret key|AGE-SECRET""-KEY-PQ-1$(rep Q 58)|1"
  "google oauth secret|s=GOC""SPX-$(rep a 28)|1"
  "resend key|k=r""e_$(rep a 8)_$(rep a 24)|1"
  "a word starting re_|re_render_the_page()|0"
  "postgres url with a password|$pg:$(rep a 20)@db:5432/x|1"
  "postgres url, local password|$pg:local-only-ratings@127.0.0.1:55432/x|0"
  "postgres url, CI password|$pg:ci-only-not-a-real-secret@postgres:5432/x|0"
  "postgres url, placeholder|$pg:%s@db:5432/x $pg:\${PW}@db/x $pg:password@host/x|0"
  "postgres url, no password|postgres://db:5432/x and mail@example.invalid|0"
)
dir=$(repo files)
for c in "${cases[@]}"; do
  IFS='|' read -r name line want <<<"$c"
  printf '%s\n' "$line" >"$dir/f.txt"
  out=$(bash "$dir/checks/secret-scan.sh" "$dir/f.txt" 2>&1)
  got=$?
  [ "$got" -eq "$want" ] || fail "$name: exit = $got, want $want; output: $out"
done

token="t=gh""p_$(rep a 36)"

dir=$(repo secret)
printf 'kind: Secret\ndata:\n  k: dg==\n' >"$dir/s.yaml"
out=$(bash "$dir/checks/secret-scan.sh" "$dir/s.yaml" 2>&1)
[ $? -eq 1 ] || fail "a Secret carrying data: passed; output: $out"

dir=$(repo many)
for i in $(seq 1 250); do echo clean >"$dir/f$i.txt"; done
echo "$token" >"$dir/last.txt"
out=$(cd "$dir" && bash checks/secret-scan.sh f*.txt last.txt 2>&1)
[ $? -eq 1 ] || fail "a token in the 251st file: passed; output: $out"

dir=$(repo all)
echo clean >"$dir/clean.txt"
git -C "$dir" add clean.txt
echo "$token" >"$dir/untracked.txt"
out=$(cd "$dir" && bash checks/secret-scan.sh --all 2>&1)
[ $? -eq 1 ] || fail "--all, a token in an untracked file: passed; output: $out"

dir=$(repo staged)
echo "$token" >"$dir/f.txt"
git -C "$dir" add f.txt
echo clean >"$dir/f.txt"
out=$(cd "$dir" && bash checks/secret-scan.sh --staged 2>&1)
[ $? -eq 1 ] || fail "staged token, clean on disk: --staged passed; output: $out"

dir=$(repo pushed)
echo "$token" >"$dir/f.txt"
git -C "$dir" add f.txt
commit_in "$dir" --no-verify -m token
echo clean >"$dir/f.txt"
zero=0000000000000000000000000000000000000000
out=$(cd "$dir" && echo "refs/heads/b $(git rev-parse HEAD) refs/heads/b $zero" | bash .githooks/pre-push 2>&1)
[ $? -eq 1 ] || fail "pushed token, clean on disk: pre-push passed; output: $out"

dir=$(repo pushed-deleted)
echo "$token" >"$dir/f.txt"
git -C "$dir" add f.txt
commit_in "$dir" --no-verify -m token
rm "$dir/f.txt"
out=$(cd "$dir" && echo "refs/heads/b $(git rev-parse HEAD) refs/heads/b $zero" | bash .githooks/pre-push 2>&1)
[ $? -eq 1 ] || fail "pushed token, file deleted on disk: pre-push passed; output: $out"

out=$(cd "$dir" && bash checks/secret-scan.sh --rev not-a-commit f.txt 2>&1)
[ $? -eq 1 ] || fail "--rev with no such commit: passed; output: $out"

dir=$work/no-git/checks
mkdir -p "$dir"
cp "$root/checks/secret-scan.sh" "$dir/"
echo "$token" >"$dir/../f.txt"
out=$(GIT_CEILING_DIRECTORIES=$work bash "$dir/secret-scan.sh" --all 2>&1)
[ $? -eq 1 ] || fail "--all outside a git repo: passed; output: $out"

testlib_finish
