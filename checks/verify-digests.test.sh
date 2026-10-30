#!/usr/bin/env bash
# Tests checks/verify-digests.sh in a throwaway repo pinning one image, against a stand-in curl that answers each
# request with the next status in its list (the last one repeating) and counts the requests.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start verify-digests.test

dir=$work/repo
mkdir -p "$dir/checks" "$dir/deploy" "$work/bin"
cp checks/verify-digests.sh "$dir/checks/"
echo "image: registry.example.invalid/team/app@sha256:$(printf '%064d' 0)" >"$dir/deploy/app.yaml"
git -C "$dir" init -q
git -C "$dir" add -A

cat >"$work/bin/curl" <<'EOF'
#!/usr/bin/env bash
n=$(($(cat "$CALLS" 2>/dev/null || echo 0) + 1))
echo "$n" >"$CALLS"
read -ra codes <<<"$CODES"
i=$((n <= ${#codes[@]} ? n - 1 : ${#codes[@]} - 1))
printf 'HTTP/2 %s\r\n\r\n' "${codes[$i]}"
EOF
printf '#!/bin/sh\n' >"$work/bin/sleep"
chmod +x "$work/bin/curl" "$work/bin/sleep"

# case: name, the statuses the registry gives in turn, the exit wanted, the requests wanted.
cases=(
  "found|200|0|1"
  "not-found-is-not-retried|404 200|1|1"
  "rate-limited-then-found|429 200|0|2"
  "server-error-then-found|503 200|0|2"
  "server-error-twice|502 502 200|1|2"
)

for c in "${cases[@]}"; do
  IFS='|' read -r name codes want calls <<<"$c"
  rm -f "$work/calls"
  out=$(cd "$dir" && PATH="$work/bin:$PATH" CODES=$codes CALLS=$work/calls GITHUB_TOKEN=unused bash checks/verify-digests.sh 2>&1)
  got=$?
  made=$(cat "$work/calls" 2>/dev/null || echo 0)
  if [ "$got" -ne "$want" ]; then
    fail "$name: exit = $got, want $want; output: $out"
  elif [ "$made" -ne "$calls" ]; then
    fail "$name: requests = $made, want $calls; output: $out"
  fi
done

testlib_finish
