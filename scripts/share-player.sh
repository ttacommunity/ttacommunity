#!/usr/bin/env bash
# Show a local site build through one Cloudflare quick tunnel, where a visitor signs in as an invented Discord
# player (README, "Let someone sign in as a test player"). scripts/dev.sh may keep running. Ctrl-C stops it all.
# Usage: scripts/share-player.sh
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# shellcheck source=scripts/lib.sh
. scripts/lib.sh
load_env

api_port=8090 site_port=4323 proxy_port=4325
work=${XDG_CACHE_HOME:-$HOME/.cache}/ttacommunity/share-player

# wait_status polls URL $1 for up to $3 seconds (default 60) until it answers with HTTP status $2, and says what
# it got.
wait_status() {
  local got=000 end=$((SECONDS + ${3:-60}))
  while [ "$SECONDS" -lt "$end" ]; do
    got=$(curl -s -o /dev/null --max-time 5 -w '%{http_code}' "$1")
    if [ "$got" = "$2" ]; then echo "  ok      $2  $1"; return 0; fi
    sleep 1
  done
  echo "  FAILED  $1 answered $got, want $2" >&2
  return 1
}

if ! command -v cloudflared >/dev/null; then
  echo "share-player.sh: cloudflared is not installed (macOS: brew install cloudflared; Linux:" \
    "https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/)" >&2
  exit 1
fi
for port in $api_port $site_port $proxy_port; do
  curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$port/"
  # 7: nothing listens there.
  if [ $? -ne 7 ]; then echo "share-player.sh: something listens on 127.0.0.1:$port; stop it first" >&2; exit 1; fi
done

mkdir -p "$work" || exit 1
(cd backend && go build -o "$work/api" ./cmd/api && go build -o "$work/shareproxy" ./cmd/shareproxy) || exit 1

pids=()
stop() { trap - EXIT INT TERM; [ ${#pids[@]} -eq 0 ] || kill "${pids[@]}" 2>/dev/null; wait; }
trap stop EXIT INT TERM

cloudflared tunnel --no-autoupdate --url "http://127.0.0.1:$proxy_port" >"$work/tunnel.log" 2>&1 & pids+=($!)
public=
for _ in $(seq 1 60); do
  public=$(grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' "$work/tunnel.log" | head -1)
  [ -n "$public" ] || ! kill -0 "${pids[0]}" 2>/dev/null && break
  sleep 1
done
[ -n "$public" ] || { echo "share-player.sh: cloudflared gave no tunnel address; see $work/tunnel.log" >&2; exit 1; }

# Only the Discord stand-in signs in; email's, if .env has it, prints its links here, so a visitor cannot finish one.
(cd backend && API_ADDR=127.0.0.1:$api_port METRICS_ADDR='' \
  SITE_ORIGIN=http://127.0.0.1:$site_port API_ORIGIN=http://127.0.0.1:$api_port \
  DISCORD_DEV_USER=local_player DISCORD_CLIENT_ID='' DISCORD_CLIENT_SECRET='' \
  ADMIN_DEV_USER='' MICROSOFT_DEV_USER='' GOOGLE_DEV_USER='' exec "$work/api") & pids+=($!)
wait_status "http://127.0.0.1:$api_port/healthz" 200 || exit 1

(cd web && export COREPACK_ENABLE_DOWNLOAD_PROMPT=0 && corepack pnpm install --frozen-lockfile --silent &&
  PUBLIC_API_URL=$public corepack pnpm --filter @ttac/site build) || exit 1
# --ignore-lock keeps astro in the foreground when an AI agent runs this.
(cd web/site && exec node node_modules/astro/bin/astro.mjs preview --ignore-lock --port "$site_port" \
  --host 127.0.0.1 --allowed-hosts "${public#https://}") & pids+=($!)
"$work/shareproxy" -public "$public" -listen "127.0.0.1:$proxy_port" \
  -api "http://127.0.0.1:$api_port" -site "http://127.0.0.1:$site_port" & pids+=($!)

ok=0
wait_status "http://127.0.0.1:$api_port/v1/admin/me" 401 || ok=1
# A new tunnel's name can take a minute or more to resolve.
wait_status "$public/blitz/" 200 180 || ok=1
wait_status "$public/v1/blitz" 200 || ok=1
wait_status "$public/v1/admin/me" 403 || ok=1
[ "$ok" = 0 ] || { echo "share-player.sh: not sharing: a check above failed" >&2; exit 1; }

echo "share-player.sh: share $public"
echo "  Anyone with it signs in as the invented Discord player local_player, on your local database."
echo "  web/site/dist holds this build until another build replaces it. Ctrl-C stops the tunnel, api, site and proxy."
wait
