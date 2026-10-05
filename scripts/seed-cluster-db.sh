#!/usr/bin/env bash
# Seed an empty ratings database from a dump, and create the api's read-only
# login. Refuses a database that already has tables. Run with no arguments
# for usage.
set -euo pipefail

cd "$(dirname "$0")/.."

usage() {
  cat >&2 <<'EOF'
Usage: scripts/seed-cluster-db.sh --host HOST [--port PORT] [--dump FILE]
       scripts/seed-cluster-db.sh --kubectl [--dump FILE]

  --host, --port  connect directly (default port 5432). Needs psql and
                  pg_restore. PGPASSWORD is the ratings owner's password;
                  RATINGS_API_PASSWORD becomes ratings_api's
  --kubectl       go through kubectl to the ttacommunity namespace instead,
                  reading both passwords from its Secrets (deploy/SECRETS.md)
  --dump          a pg_dump -Fc file; without one, the local compose
                  database is dumped
EOF
  exit 64
}

mode="" host="" port=5432 dump=""
while [ $# -gt 0 ]; do
  case $1 in
    --host) mode=direct; host=${2:?}; shift ;;
    --port) port=${2:?}; shift ;;
    --kubectl) mode=kubectl ;;
    --dump) dump=${2:?}; shift ;;
    *) usage ;;
  esac
  shift
done
[ -n "$mode" ] || usage

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [ "$mode" = direct ]; then
  : "${PGPASSWORD:?set PGPASSWORD to the password of the ratings owner}"
  : "${RATINGS_API_PASSWORD:?set RATINGS_API_PASSWORD to the password for ratings_api}"
  export PGPASSWORD RATINGS_API_PASSWORD
  sql() { psql -X -q -v ON_ERROR_STOP=1 -h "$host" -p "$port" -U ratings -d ratings "$@"; }
else
  ns=(-n ttacommunity)
  pod=(exec -i deploy/ratings-db --)
  sql() { kubectl "${ns[@]}" "${pod[@]}" psql -X -q -v ON_ERROR_STOP=1 -U ratings -d ratings "$@"; }
  kubectl "${ns[@]}" rollout status deploy/ratings-db --timeout=60s
fi

if [ -z "$dump" ]; then
  if docker compose version >/dev/null 2>&1; then
    compose=(docker compose)
  elif podman compose version >/dev/null 2>&1; then
    compose=(podman compose)
  else
    echo "seed-cluster-db.sh: needs --dump, or docker/podman compose to dump the local database" >&2
    exit 1
  fi
  dump=$work/ratings.dump
  echo "dumping the local database"
  "${compose[@]}" exec -T ratings-db pg_dump -U ratings -d ratings -Fc > "$dump"
fi
[ -s "$dump" ] || { echo "seed-cluster-db.sh: $dump is empty or missing" >&2; exit 1; }

tables=$(sql -Atc "select count(*) from pg_tables where schemaname = 'public'")
if [ "$tables" != 0 ]; then
  echo "seed-cluster-db.sh: the database already has $tables tables; refusing" >&2
  exit 1
fi

# Roles are per server, so a dump does not carry them. Passwords never go on
# a command line.
roles="DO \$\$ BEGIN CREATE ROLE ratings_read NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
DO \$\$ BEGIN CREATE ROLE ratings_admin NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
DO \$\$ BEGIN CREATE ROLE ratings_api LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
GRANT ratings_read TO ratings_api;"
echo "creating roles"
if [ "$mode" = direct ]; then
  sql <<SQL
$roles
\getenv pw RATINGS_API_PASSWORD
SELECT format('ALTER ROLE ratings_api PASSWORD %L', :'pw') \gexec
SQL
else
  pw=$(kubectl "${ns[@]}" get secret ratings-api -o jsonpath='{.data.password}' | base64 -d)
  [[ $pw =~ ^[0-9a-f]{32,}$ ]] || { echo "seed-cluster-db.sh: ratings-api's password must be hex" >&2; exit 1; }
  sql <<SQL
$roles
ALTER ROLE ratings_api PASSWORD '$pw';
SQL
  unset pw
fi

if [ "$mode" = direct ]; then
  echo "restoring"
  pg_restore --exit-on-error --jobs=4 -h "$host" -p "$port" -U ratings -d ratings "$dump"
else
  # A long kubectl exec stream through the API server stalls, so the dump goes
  # over in short requests and is restored inside the pod.
  remote=/tmp/ratings.dump
  echo "copying $(du -h "$dump" | cut -f1) into the pod"
  split -b 8M -d -a 4 "$dump" "$work/part."
  kubectl "${ns[@]}" "${pod[@]}" sh -c ": > $remote"
  for part in "$work"/part.*; do
    for try in 1 2 3; do
      if timeout 50 kubectl "${ns[@]}" "${pod[@]}" sh -c "cat > $remote.part && cat $remote.part >> $remote"; then
        break
      fi
      [ "$try" = 3 ] && { echo "seed-cluster-db.sh: copying $part failed 3 times" >&2; exit 1; }
    done < "$part"
  done
  kubectl "${ns[@]}" "${pod[@]}" rm -f "$remote.part"
  want=$(sha256sum < "$dump" | cut -d' ' -f1)
  got=$(kubectl "${ns[@]}" "${pod[@]}" sha256sum "$remote" | cut -d' ' -f1)
  [ "$want" = "$got" ] || { echo "seed-cluster-db.sh: the copy's checksum differs" >&2; exit 1; }

  echo "restoring inside the pod"
  kubectl "${ns[@]}" "${pod[@]}" sh -c "setsid nohup sh -c 'pg_restore --verbose --exit-on-error -U ratings -d ratings $remote > /tmp/restore.log 2>&1; echo exit=\$? >> /tmp/restore.log' > /dev/null 2>&1 < /dev/null &"
  until result=$(kubectl "${ns[@]}" "${pod[@]}" sh -c 'grep "^exit=" /tmp/restore.log' 2>/dev/null); do
    sleep 15
  done
  kubectl "${ns[@]}" "${pod[@]}" rm -f "$remote"
  if [ "$result" != exit=0 ]; then
    echo "seed-cluster-db.sh: pg_restore failed; see /tmp/restore.log in the ratings-db pod" >&2
    exit 1
  fi
fi

sql -Atc "select 'players: ' || count(*) from players union all select 'rating runs: ' || count(*) from rating_runs"
