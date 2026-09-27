#!/usr/bin/env bash
# Copy a ratings database dump into the cluster's database, and give the api's
# read-only login the password in the ratings-api Secret.
# The cluster database must be new and empty: this refuses otherwise.
#
# Usage: scripts/seed-cluster-db.sh [dump]
#   dump  a pg_dump -Fc file; without one, the local compose database is dumped
set -euo pipefail

cd "$(dirname "$0")/.."

ns=(-n ttacommunity)
pod=(exec -i deploy/ratings-db --)
remote=/tmp/ratings.dump
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

dump=${1:-}
if [ -z "$dump" ]; then
  if docker compose version >/dev/null 2>&1; then
    compose=(docker compose)
  elif podman compose version >/dev/null 2>&1; then
    compose=(podman compose)
  else
    echo "seed-cluster-db.sh: needs docker compose, or podman with a compose provider" >&2
    exit 1
  fi
  dump=$work/ratings.dump
  echo "dumping the local database"
  "${compose[@]}" exec -T ratings-db pg_dump -U ratings -d ratings -Fc > "$dump"
fi
[ -s "$dump" ] || { echo "seed-cluster-db.sh: $dump is empty or missing" >&2; exit 1; }

kubectl "${ns[@]}" rollout status deploy/ratings-db --timeout=60s
tables=$(kubectl "${ns[@]}" "${pod[@]}" psql -U ratings -d ratings -Atc \
  "select count(*) from pg_tables where schemaname = 'public'")
if [ "$tables" != 0 ]; then
  echo "seed-cluster-db.sh: the cluster database already has $tables tables; refusing" >&2
  exit 1
fi

pw=$(kubectl "${ns[@]}" get secret ratings-api -o jsonpath='{.data.password}' | base64 -d)
if [[ ! $pw =~ ^[0-9a-f]{32,}$ ]]; then
  echo "seed-cluster-db.sh: ratings-api's password must be hex, as SECRETS.md makes it" >&2
  exit 1
fi

# Roles are per server, so a dump does not carry them. The password goes on
# stdin, never in a command line.
echo "creating roles"
kubectl "${ns[@]}" "${pod[@]}" psql -q -v ON_ERROR_STOP=1 -U ratings -d ratings <<SQL
DO \$\$ BEGIN CREATE ROLE ratings_read NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
DO \$\$ BEGIN CREATE ROLE ratings_api LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
ALTER ROLE ratings_api PASSWORD '$pw';
GRANT ratings_read TO ratings_api;
SQL
unset pw

# A long kubectl exec stream through the API server stalls, so the dump goes
# over in short requests and is restored inside the pod.
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

kubectl "${ns[@]}" "${pod[@]}" psql -U ratings -d ratings -Atc \
  "select 'players: ' || count(*) from players union all select 'rating runs: ' || count(*) from rating_runs"
