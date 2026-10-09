#!/usr/bin/env bash
# Create the dev site's database, ratings_dev, on the cluster's ratings server as a copy of production's, with
# its grants, owner and read-only logins. Passwords come from the Secrets in deploy/SECRETS.md.
set -euo pipefail

cd "$(dirname "$0")/.."
usage() {
  cat >&2 <<'EOF'
Usage: scripts/seed-dev-db.sh [--replace] [--latest-run | --no-runs | --all-runs]
  --latest-run  inputs and production's current published run (the default)
  --no-runs     inputs only, as a baseline holds them: nothing published until an engine run
  --all-runs    everything, every run
  --replace     drop ratings_dev first (otherwise an existing one is refused)
EOF
  exit 64
}
mode=latest-run replace=0
for arg in "$@"; do
  case "$arg" in
    --latest-run | --no-runs | --all-runs) mode=${arg#--} ;;
    --replace) replace=1 ;;
    *) usage ;;
  esac
done

ns=(-n ttacommunity)
pod=(exec -i deploy/ratings-db --)
sql() { kubectl "${ns[@]}" "${pod[@]}" psql -X -q -v ON_ERROR_STOP=1 -U ratings "$@"; }
secret() { kubectl "${ns[@]}" get secret "$1" -o jsonpath="{.data.$2}" | base64 -d; }

kubectl "${ns[@]}" rollout status deploy/ratings-db --timeout=60s

if [ "$replace" = 1 ]; then
  echo "dropping ratings_dev"
  sql -d ratings -c "DROP DATABASE IF EXISTS ratings_dev WITH (FORCE)"
elif [ "$(sql -d ratings -Atc "select count(*) from pg_database where datname = 'ratings_dev'")" != 0 ]; then
  echo "seed-dev-db.sh: ratings_dev already exists; refusing (--replace drops it)" >&2
  exit 1
fi

owner_url=$(secret ratings-dev-engine database-url)
owner_pw=${owner_url#postgres://ratings_dev:}
owner_pw=${owner_pw%%@*}
api_pw=$(secret ratings-dev-api password)
for pw in "$owner_pw" "$api_pw"; do
  [[ $pw =~ ^[0-9a-f]{32,}$ ]] || { echo "seed-dev-db.sh: the dev Secrets' passwords must be hex" >&2; exit 1; }
done

# ratings_dev owns only its own database, so the dev engine cannot write the parity one.
echo "creating roles and the database"
sql -d ratings <<SQL
DO \$\$ BEGIN CREATE ROLE ratings_dev LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
DO \$\$ BEGIN CREATE ROLE ratings_dev_api LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
GRANT ratings_read TO ratings_dev_api;
ALTER ROLE ratings_dev PASSWORD '$owner_pw';
ALTER ROLE ratings_dev_api PASSWORD '$api_pw';
CREATE DATABASE ratings_dev OWNER ratings_dev;
SQL
unset owner_url owner_pw api_pw pw

# A long kubectl exec stream stalls, so the copy runs detached inside the pod.
echo "copying ratings into ratings_dev inside the pod ($mode)"
# Which tables are inputs is the load job's to say, not this script's.
(cd backend && go run ./cmd/load -input-tables) | kubectl "${ns[@]}" "${pod[@]}" sh -c 'cat > /tmp/input-tables'
kubectl "${ns[@]}" "${pod[@]}" sh -c 'cat > /tmp/copy-ratings-db.sh' < scripts/copy-ratings-db.sh
kubectl "${ns[@]}" "${pod[@]}" rm -f /tmp/dev-restore.log
kubectl "${ns[@]}" "${pod[@]}" sh -c "setsid nohup sh -c 'sh /tmp/copy-ratings-db.sh ratings ratings_dev ratings_dev $mode /tmp/input-tables > /tmp/dev-restore.log 2>&1; echo exit=\$? >> /tmp/dev-restore.log' > /dev/null 2>&1 < /dev/null &"
until result=$(kubectl "${ns[@]}" "${pod[@]}" sh -c 'grep "^exit=" /tmp/dev-restore.log' 2>/dev/null); do
  sleep 15
done
if [ "$result" != exit=0 ]; then
  echo "seed-dev-db.sh: the copy failed; see /tmp/dev-restore.log in the ratings-db pod" >&2
  exit 1
fi

# The copy keeps production's grants; ratings_dev gets its own default privileges for the tables its migrations create.
sql -d ratings_dev <<'SQL'
GRANT USAGE ON SCHEMA public TO ratings_read;
ALTER DEFAULT PRIVILEGES FOR ROLE ratings_dev IN SCHEMA public GRANT SELECT ON TABLES TO ratings_read;
SQL
sql -d ratings_dev -Atc "select 'players: ' || count(*) from players union all select 'rating runs: ' || count(*) from rating_runs union all select 'current run: ' || coalesce((select run_id::text from current_run), 'none')"
