#!/usr/bin/env bash
# Back up what exists in one place only: the Secrets in the ttacommunity namespace and the Cloudflare settings
# that are not in git. Each is exported as JSON, encrypted with age to the backup key and uploaded to the R2
# bucket ttacommunity-config-backup, only when its SHA-256 differs from the newest copy's. Nothing unencrypted
# touches the disk. Usage: scripts/backup-config.sh [--check]   (--check: show what would upload, upload nothing)
set -euo pipefail

cd "$(dirname "$0")/.."
check=0
case "${1:-}" in
  "") ;;
  --check) check=1 ;;
  *) echo "Usage: scripts/backup-config.sh [--check]" >&2; exit 64 ;;
esac

ns=ttacommunity
zone=${BACKUP_CONFIG_ZONE:-ttacommunity.org}
bucket=${BACKUP_CONFIG_BUCKET:-ttacommunity-config-backup}
recipient=${BACKUP_CONFIG_AGE_RECIPIENT:-age1chparhxyn3kvlr245t8u9zvm9agar9v995dyptuygepuvmeujazqkre4r9}
jurisdiction=eu

fail() { echo "backup-config.sh: $*" >&2; exit 1; }
for tool in age kubectl cf rclone jq; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done
secret() { kubectl -n "$ns" get secret backup-config-r2 -o jsonpath="{.data.$1}" | base64 -d; }
# The remote comes from the environment alone, never from a maintainer's rclone.conf.
export RCLONE_CONFIG=/dev/null RCLONE_CONFIG_R2_TYPE=s3 RCLONE_CONFIG_R2_PROVIDER=Cloudflare
RCLONE_CONFIG_R2_ENDPOINT=$(secret endpoint) || fail "cannot read the Secret backup-config-r2"
RCLONE_CONFIG_R2_ACCESS_KEY_ID=$(secret access-key-id)
RCLONE_CONFIG_R2_SECRET_ACCESS_KEY=$(secret secret-access-key)
export RCLONE_CONFIG_R2_ENDPOINT RCLONE_CONFIG_R2_ACCESS_KEY_ID RCLONE_CONFIG_R2_SECRET_ACCESS_KEY
[ -n "$RCLONE_CONFIG_R2_ENDPOINT" ] && [ -n "$RCLONE_CONFIG_R2_ACCESS_KEY_ID" ] && [ -n "$RCLONE_CONFIG_R2_SECRET_ACCESS_KEY" ] ||
  fail "the Secret backup-config-r2 is missing a key"
# Only the cf profile bound to this repo (cf auth list) reaches the ttacommunity account.
cf auth whoami >/dev/null 2>&1 || fail "cf is not signed in"
rclone_r2() { rclone "$@" --s3-no-check-bucket --s3-no-head --retries 3; }

# read_cf runs a cf read, retrying the API's intermittent "Envelope Error", and fails when it never answers JSON.
read_cf() {
  local out try
  for try in 1 2 3; do
    if out=$(cf "$@" 2>/dev/null) && jq -e . >/dev/null 2>&1 <<<"$out"; then
      printf '%s' "$out"
      return 0
    fi
    [ "$try" = 3 ] || sleep 2
  done
  fail "cf $* gave no JSON after 3 tries"
}

# export_secrets prints every Secret in the namespace as a kubectl-applicable List: name, type and data only.
export_secrets() {
  kubectl -n "$ns" get secrets -o json | jq -S '{apiVersion: "v1", kind: "List", items: [.items[] |
    {apiVersion, kind, type, metadata: {name: .metadata.name, namespace: .metadata.namespace}, data: (.data // {})}]
    | sort_by(.metadata.name)}'
}

# export_cloudflare prints one JSON object of every Cloudflare setting cf can read; docs/architecture.md names the
# rest. Lists ask for 50 a page: the account has a handful of each. A read that fails aborts the export.
export_cloudflare() {
  local out workers worker tag triggers trigger id name v
  out=$(jq -n --arg z "$zone" '{zone: $z, rulesets: {}, build_variables: {}, r2_buckets: {}}')
  # add stores a cf read at a dotted path of the export.
  add() {
    local path=$1; shift
    v=$(read_cf "$@") || exit 1
    out=$(jq --arg p "$path" --argjson v "$v" 'setpath($p | split("."); $v)' <<<"$out")
  }
  add organization zero-trust organizations list
  add access_applications zero-trust access applications list --per-page 50
  add access_policies zero-trust access policies list --per-page 50
  add access_groups zero-trust access groups list --per-page 50
  add access_service_tokens zero-trust access service-tokens list --per-page 50
  add identity_providers zero-trust identity-providers list --per-page 50
  add dns_records dns records list -z "$zone" --per-page 50
  add turnstile_widgets turnstile widgets list --per-page 50
  add email_routing.settings email-routing settings get -z "$zone"
  add email_routing.rules email-routing rules list-account -z "$zone" --per-page 50
  add email_routing.catch_all email-routing rules catch-all get -z "$zone"
  add email_routing.addresses email-routing addresses list --per-page 50
  for id in $(read_cf rulesets account-rulesets list -z "$zone" | jq -r '.[] | select(.kind == "zone") | .id'); do
    add "rulesets.$id" rulesets account-rulesets get "$id" -z "$zone"
  done
  add workers workers scripts search
  workers=$(jq -r '.workers[] | .id + " " + .script_name' <<<"$out")
  triggers=""
  while read -r worker tag; do
    add "build_triggers.$tag" builds triggers list --external-script-id "$worker"
    # A Worker deployed by hand has no build configuration (404).
    if [ "$(jq -r --arg t "$tag" '.build_triggers[$t] | length' <<<"$out")" != 0 ]; then
      add "builds.$tag" builds workers get "$worker"
      triggers="$triggers $(jq -r --arg t "$tag" '.build_triggers[$t][].trigger_uuid' <<<"$out")"
    fi
  done <<<"$workers"
  for trigger in $triggers; do
    add "build_variables.$trigger" builds triggers environment-variables list --trigger-uuid "$trigger"
  done
  for name in $(read_cf r2 buckets list --cf-r2-jurisdiction $jurisdiction | jq -r '.buckets[].name'); do
    add "r2_buckets.$name.lifecycle" r2 buckets lifecycle get "$name" --cf-r2-jurisdiction $jurisdiction
    add "r2_buckets.$name.locks" r2 buckets locks get "$name" --cf-r2-jurisdiction $jurisdiction
  done
  jq -S . <<<"$out"
}

# newest_sha prints the SHA-256 recorded on the newest copy under a prefix, or nothing when there is none.
newest_sha() {
  rclone_r2 lsjson "r2:$bucket/$1" -M 2>/dev/null | jq -r 'map(select(.IsDir | not)) | sort_by(.Path) | last | .Metadata.sha256 // empty'
}

# upload_if_changed exports one kind (secrets or cloudflare) with the given function, and uploads it encrypted
# when its fingerprint differs from the newest copy's. The export lives in memory only.
upload_if_changed() {
  local prefix=$1 export=$2 json sha newest key
  json=$($export)
  sha=$(printf '%s' "$json" | sha256sum | cut -d' ' -f1)
  newest=$(newest_sha "$prefix")
  if [ "$sha" = "$newest" ]; then
    echo "$prefix: unchanged ($sha)"
    return 0
  fi
  key="$prefix/$(date -u +%Y-%m-%dT%H%M%SZ).json.age"
  if [ "$check" = 1 ]; then
    echo "$prefix: would upload $key ($sha; newest copy ${newest:-none})"
    return 0
  fi
  printf '%s' "$json" | age -r "$recipient" | rclone_r2 rcat "r2:$bucket/$key" -M --metadata-set "sha256=$sha"
  echo "$prefix: uploaded $key ($sha)"
}

upload_if_changed secrets export_secrets
upload_if_changed cloudflare export_cloudflare
