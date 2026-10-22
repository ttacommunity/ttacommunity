#!/usr/bin/env bash
# Tests scripts/backup-config.sh against stub age, kubectl, cf and rclone: it refuses before any upload when a
# tool or the token is missing, uploads both copies the first time and nothing when nothing changed, uploads only
# the Secrets copy when a Secret changed, and never hands rclone anything age did not encrypt.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
stubs=$work/stubs
mkdir -p "$stubs"
export STUB_LOG=$work/rclone.log STUB_SECRETS=$work/secrets.json STUB_NEWEST=$work/newest
export BACKUP_CONFIG_BUCKET=stub-bucket BACKUP_CONFIG_AGE_RECIPIENT=age1stub

cat >"$stubs/age" <<'EOF'
#!/usr/bin/env bash
printf 'age-encryption.org/v1 stub\n'; cat
EOF
cat >"$stubs/kubectl" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"get secret backup-config-r2"*) [ "${STUB_NO_TOKEN:-}" = 1 ] && exit 1; printf 'c3R1Yg==' ;;
  *"get secrets -o json"*) cat "$STUB_SECRETS" ;;
  *) echo "kubectl stub: unexpected $*" >&2; exit 1 ;;
esac
EOF
cat >"$stubs/cf" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "auth whoami") echo '{}' ;;
  "workers scripts search") echo '[{"id":"tag1","script_name":"w1"}]' ;;
  "builds triggers list"*) echo '[{"trigger_uuid":"t1"}]' ;;
  "r2 buckets list"*) echo '{"buckets":[{"name":"b1"}]}' ;;
  "rulesets account-rulesets list"*) echo '[{"id":"r1","kind":"zone"}]' ;;
  *) echo "{\"stub\":\"$1 $2\"}" ;;
esac
EOF
cat >"$stubs/rclone" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  lsjson) cat "$STUB_NEWEST-${2##*/}.json" ;;
  rcat)
    read -r first
    [ "$first" = "age-encryption.org/v1 stub" ] || { echo "rclone stub: rcat got unencrypted data" >&2; exit 1; }
    cat >/dev/null
    path=$2 meta=""
    while [ $# -gt 0 ]; do [ "$1" = --metadata-set ] && meta=$2; shift; done
    echo "rcat $path $meta" >>"$STUB_LOG" ;;
  *) echo "rclone stub: unexpected $*" >&2; exit 1 ;;
esac
EOF
chmod +x "$stubs"/*
# Only the stubs and the system's utilities: a real tool further down PATH would stand in for a removed stub.
saved_path=$PATH
export PATH=$stubs:/usr/bin:/bin
for tool in age kubectl cf rclone; do
  real=$(PATH=/usr/bin:/bin command -v "$tool") && { echo "backup-config.test: $tool is installed at $real, where the test cannot hide it" >&2; exit 1; }
done

echo '{"items":[{"apiVersion":"v1","kind":"Secret","type":"Opaque","metadata":{"name":"a","namespace":"ttacommunity","uid":"x"},"data":{"k":"dg=="}}]}' >"$STUB_SECRETS"
echo '[]' >"$STUB_NEWEST-secrets.json"
echo '[]' >"$STUB_NEWEST-cloudflare.json"
: >"$STUB_LOG"

failed=0
# expect runs the script and compares its exit code and the uploads rclone saw (as "secrets cloudflare" counts).
expect() {
  local name=$1 want_exit=$2 want_uploads=$3 out got uploads
  : >"$STUB_LOG"
  out=$("${script[@]}" 2>&1)
  got=$?
  uploads="$(grep -c 'rcat r2:stub-bucket/secrets/' "$STUB_LOG") $(grep -c 'rcat r2:stub-bucket/cloudflare/' "$STUB_LOG")"
  if [ "$got" -ne "$want_exit" ] || [ "$uploads" != "$want_uploads" ]; then
    echo "  ${script[-1]##*/} $name: exit = $got, uploads = $uploads; want exit $want_exit, uploads $want_uploads; output: $out" >&2
    failed=1
  fi
}

# cases runs every case against $script. The twin must give the bash script's fingerprints, so the newest copies
# the bash run made say "nothing changed" to the twin too.
cases() {
  local tool prefix
  for tool in age kubectl cf rclone; do
    mv "$stubs/$tool" "$stubs/$tool.off"
    expect "no $tool" 1 "0 0"
    mv "$stubs/$tool.off" "$stubs/$tool"
  done
  STUB_NO_TOKEN=1 expect "no token" 1 "0 0"
  echo '[]' >"$STUB_NEWEST-secrets.json"
  echo '[]' >"$STUB_NEWEST-cloudflare.json"
  expect "first run" 0 "1 1"
  if [ -z "$first_log" ]; then
    cp "$STUB_LOG" "$first_log_file"
    first_log=$first_log_file
  fi
  for prefix in secrets cloudflare; do
    grep "rcat r2:stub-bucket/$prefix/" "$first_log" | sed "s|^rcat r2:stub-bucket/$prefix/||; s|sha256=||" |
      jq -R -s 'split("\n") | map(select(. != "") | split(" ") | {Path: .[0], IsDir: false, Metadata: {sha256: .[1]}})' >"$STUB_NEWEST-$prefix.json"
  done
  expect "nothing changed" 0 "0 0"
  sed -i 's/"dg=="/"dw=="/' "$STUB_SECRETS"
  expect "one Secret changed" 0 "1 0"
  sed -i 's/"dw=="/"dg=="/' "$STUB_SECRETS"
}

first_log="" first_log_file=$work/first.log
script=(bash scripts/backup-config.sh)
cases
# The twin, by its real binary: pwsh's mise shim would put the real tools back on PATH.
ran="the bash script"
if pwsh=$(PATH=$saved_path mise which pwsh 2>/dev/null || PATH=$saved_path command -v pwsh); then
  script=(env DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1 "$pwsh" -NoProfile -File scripts/backup-config.ps1)
  cases
  ran="both scripts"
else
  echo "  no pwsh: the .ps1 twin was not run" >&2
fi

if [ "$failed" -ne 0 ]; then
  echo "backup-config.test: failed" >&2
  exit 1
fi
echo "backup-config.test: every case as expected, $ran"
