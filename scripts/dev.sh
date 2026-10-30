#!/usr/bin/env bash
# Start everything for local development and check that it answers.
#
# Usage: scripts/dev.sh            all in containers, detached
#        scripts/dev.sh --native   Postgres in containers; api, site run here
#        scripts/dev.sh down       stop the containers (data volumes are kept)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

# shellcheck source=scripts/lib.sh
. scripts/lib.sh
find_compose || exit 1
load_env
# .env is made once, so a setting added to .env.example since is missing from an older one.
missing=$(LC_ALL=C comm -23 \
  <(sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' .env.example | LC_ALL=C sort -u) \
  <(sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' .env | LC_ALL=C sort -u) | paste -sd, -)
if [ -n "$missing" ]; then
  echo "dev.sh: .env has no ${missing//,/, }, which .env.example sets; copy those lines across" >&2
fi

# api_login creates or updates the api's login $2 with password $3, in role $1, which the migrations grant to.
api_login() {
  # shellcheck disable=SC2016 # $PW expands inside the container, not here
  login_sql "$1" "$2" | "${compose[@]}" exec -T -e PW="$3" ratings-db \
    sh -c 'psql -q -v ON_ERROR_STOP=1 -v pw="$PW" -U ratings -d ratings' || { echo "dev.sh: could not create the $2 login" >&2; return 1; }
}

# api_logins creates the api's read-only, admin and player logins.
api_logins() {
  api_login ratings_read ratings_api "$RATINGS_READ_DB_PASSWORD" &&
    api_login ratings_admin ratings_admin_api "$RATINGS_ADMIN_DB_PASSWORD" &&
    api_login ratings_player ratings_player_api "$RATINGS_PLAYER_DB_PASSWORD"
}

# wait_for polls URL $2 for up to 90s and reports it under name $1.
wait_for() {
  for _ in $(seq 1 90); do
    if curl -fs -o /dev/null "$2"; then echo "  up      $1  $2"; return 0; fi
    sleep 1
  done
  echo "  DOWN    $1  $2 did not answer within 90s" >&2
  return 1
}

# fonts_notice says, without holding up the start, when Google Fonts has newer
# fonts than the committed ones. Offline, it says nothing.
fonts_notice() {
  command -v node >/dev/null 2>&1 || return 0
  {
    out=$(cd web/site && timeout 20 node scripts/fonts.mjs check 2>/dev/null) || [ -z "$out" ] || echo "dev.sh: ${out%%$'\n'*}"
  } &
}
[ "${1:-}" = down ] || fonts_notice

case "${1:-}" in
  down)
    "${compose[@]}" --profile engine down
    exit $?
    ;;
  --native)
    "${compose[@]}" up -d --wait ratings-db || exit 1
    api_logins || exit 1
    # Each process gets its own group, so stopping it also stops what it spawned.
    # --ignore-lock stops astro detaching into a daemon when an AI agent runs this.
    groups=()
    stop() { trap - EXIT INT TERM; for g in "${groups[@]}"; do kill -TERM -- "-$g" 2>/dev/null; done; wait; }
    trap stop EXIT INT TERM
    (cd web && COREPACK_ENABLE_DOWNLOAD_PROMPT=0 corepack pnpm install --frozen-lockfile --silent) || exit 1
    setsid bash -c 'cd backend && exec go run ./cmd/api' & groups+=($!)
    setsid bash -c 'cd web/site && exec ./node_modules/.bin/astro dev --ignore-lock' & groups+=($!)
    setsid bash -c 'cd web/admin && exec ./node_modules/.bin/astro dev --ignore-lock' & groups+=($!)
    ok=0
    wait_for api http://127.0.0.1:8080/healthz || ok=1
    wait_for site http://127.0.0.1:4321/ || ok=1
    wait_for admin http://127.0.0.1:4322/ || ok=1
    [ "$ok" = 0 ] || exit 1
    echo "dev.sh: running natively. Ctrl-C stops api, site and admin; 'scripts/dev.sh down' stops Postgres."
    wait
    ;;
  "")
    "${compose[@]}" up -d --wait ratings-db || exit 1
    api_logins || exit 1
    "${compose[@]}" up -d --build --wait api || exit 1
    "${compose[@]}" up -d site || exit 1
    ok=0
    "${compose[@]}" --profile engine run --rm --build engine || { echo "  FAILED  engine run" >&2; ok=1; }
    wait_for api http://127.0.0.1:8080/healthz || ok=1
    wait_for site http://127.0.0.1:4321/ || ok=1
    "${compose[@]}" ps
    [ "$ok" = 0 ] || { echo "dev.sh: something did not start. Logs: ${compose[*]} logs" >&2; exit 1; }
    echo "dev.sh: all up. Site http://127.0.0.1:4321  API http://127.0.0.1:8080  Stop: scripts/dev.sh down"
    ;;
  *)
    echo "unknown argument: $1" >&2
    exit 64
    ;;
esac
