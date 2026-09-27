#!/usr/bin/env bash
# Copy the local ratings database into the cluster's, and give the api's
# read-only login the password in the ratings-api Secret.
# The cluster database must be new and empty: this refuses otherwise.
#
# Usage: scripts/seed-cluster-db.sh
set -euo pipefail

cd "$(dirname "$0")/.."

ns=(-n ttacommunity)
pod=(exec -i deploy/ratings-db --)

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif podman compose version >/dev/null 2>&1; then
  compose=(podman compose)
else
  echo "seed-cluster-db.sh: needs docker compose, or podman with a compose provider" >&2
  exit 1
fi

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
CREATE ROLE ratings_read NOLOGIN;
CREATE ROLE ratings_api LOGIN PASSWORD '$pw';
GRANT ratings_read TO ratings_api;
SQL
unset pw

echo "copying the local database (a few minutes)"
"${compose[@]}" exec -T ratings-db pg_dump -U ratings -d ratings -Fc |
  kubectl "${ns[@]}" "${pod[@]}" pg_restore -U ratings -d ratings --exit-on-error

kubectl "${ns[@]}" "${pod[@]}" psql -U ratings -d ratings -Atc \
  "select 'rating runs: ' || count(*) from rating_runs"
