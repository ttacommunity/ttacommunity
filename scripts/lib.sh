# shellcheck shell=bash
# Shared by the scripts: finding compose, loading .env and the SQL for an api login. Source it from the
# repository's root: . scripts/lib.sh

# find_compose sets the array compose to docker compose or podman compose, or says why neither works.
find_compose() {
  # shellcheck disable=SC2034 # compose is for the script that sources this
  if docker compose version >/dev/null 2>&1; then
    compose=(docker compose)
  elif podman compose version >/dev/null 2>&1; then
    compose=(podman compose)
    if ! podman info --format '{{.Host.RemoteSocket.Exists}}' 2>/dev/null | grep -q true; then
      echo "${0##*/}: podman's API socket is off. Start it: systemctl --user enable --now podman.socket" >&2
      return 1
    fi
  else
    echo "${0##*/}: needs docker compose, or podman with a compose provider" >&2
    return 1
  fi
}

# load_env exports .env's settings, creating .env from .env.example first when there is none.
load_env() {
  if [ ! -f .env ]; then
    cp .env.example .env
    echo "${0##*/}: created .env from .env.example (local-only values)"
  fi
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
}

# login_sql prints SQL that creates role $1 (no login) and login $2 in it when missing, and sets $2's password
# to psql's variable pw, which the caller sets.
login_sql() {
  cat <<SQL
DO \$\$ BEGIN CREATE ROLE $1 NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
DO \$\$ BEGIN CREATE ROLE $2 LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;
SELECT format('ALTER ROLE %I PASSWORD %L', '$2', :'pw') \gexec
GRANT $1 TO $2;
SQL
}
