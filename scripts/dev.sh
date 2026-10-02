#!/usr/bin/env bash
# Start everything for local development and check that it answers.
#
# Usage: scripts/dev.sh            all in containers, detached
#        scripts/dev.sh --native   Postgres in containers; api, worker, site run here
#        scripts/dev.sh down       stop the containers (data volumes are kept)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif podman compose version >/dev/null 2>&1; then
  compose=(podman compose)
  if ! podman info --format '{{.Host.RemoteSocket.Exists}}' 2>/dev/null | grep -q true; then
    echo "dev.sh: podman's API socket is off. Start it: systemctl --user enable --now podman.socket" >&2
    exit 1
  fi
else
  echo "dev.sh: needs docker compose, or podman with a compose provider" >&2
  exit 1
fi

if [ ! -f .env ]; then
  cp .env.example .env
  echo "dev.sh: created .env from .env.example (local-only values)"
fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a

# ratings_read_login creates or updates the api's read-only login, ratings_api.
# The ratings_read role it joins is the one migration 00004 grants SELECT to.
ratings_read_login() {
  # shellcheck disable=SC2016 # $PW expands inside the container, not here
  "${compose[@]}" exec -T -e PW="$RATINGS_READ_DB_PASSWORD" ratings-db \
    sh -c 'psql -q -v ON_ERROR_STOP=1 -v pw="$PW" -U ratings -d ratings' <<'SQL' || { echo "dev.sh: could not create the ratings_api login" >&2; return 1; }
DO $$ BEGIN CREATE ROLE ratings_read NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE ROLE ratings_api LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
SELECT format('ALTER ROLE ratings_api PASSWORD %L', :'pw') \gexec
GRANT ratings_read TO ratings_api;
SQL
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
    "${compose[@]}" up -d --wait ratings-db app-db || exit 1
    ratings_read_login || exit 1
    # Each process gets its own group, so stopping it also stops what it spawned.
    # --ignore-lock stops astro detaching into a daemon when an AI agent runs this.
    groups=()
    stop() { trap - EXIT INT TERM; for g in "${groups[@]}"; do kill -TERM -- "-$g" 2>/dev/null; done; wait; }
    trap stop EXIT INT TERM
    setsid bash -c 'cd backend && exec go run ./cmd/api' & groups+=($!)
    setsid bash -c 'cd backend && exec go run ./cmd/worker' & groups+=($!)
    setsid bash -c 'cd web && COREPACK_ENABLE_DOWNLOAD_PROMPT=0 corepack pnpm install --frozen-lockfile --silent &&
      cd site && exec ./node_modules/.bin/astro dev --ignore-lock' & groups+=($!)
    ok=0
    wait_for api http://127.0.0.1:8080/healthz || ok=1
    wait_for site http://127.0.0.1:4321/ || ok=1
    [ "$ok" = 0 ] || exit 1
    echo "dev.sh: running natively. Ctrl-C stops api, worker and site; 'scripts/dev.sh down' stops Postgres."
    wait
    ;;
  "")
    "${compose[@]}" up -d --wait ratings-db app-db || exit 1
    ratings_read_login || exit 1
    "${compose[@]}" up -d --build --wait api worker || exit 1
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
