#!/usr/bin/env bash
# Seed the ratings database from data kept outside the repo: a baseline export
# (a folder with manifest.json, restored whole) or a directory of match CSVs.
#
# Usage: scripts/ratings-db.sh [--native] [--reset] <dir>
#   --native  run the load job with local Go instead of in a container
#   --reset   delete every match, refusal and rating run first (match CSVs only)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

native=0
args=()
dir=""
while [ $# -gt 0 ]; do
  case $1 in
    --native) native=1 ;;
    --reset) args+=(-reset) ;;
    -*) echo "unknown argument: $1" >&2; exit 64 ;;
    *) [ -z "$dir" ] || { echo "one directory only" >&2; exit 64; }; dir=$1 ;;
  esac
  shift
done
if [ -z "$dir" ] || [ ! -d "$dir" ]; then
  echo "usage: scripts/ratings-db.sh [--native] [--reset] <baseline export or directory of match CSVs>" >&2
  exit 64
fi
dir=$(cd "$dir" && pwd -P)
restore=0
if [ -f "$dir/manifest.json" ]; then
  restore=1
  if [ ${#args[@]} -gt 0 ]; then
    echo "ratings-db.sh: a baseline is restored into an empty database; to start again, remove the database's volume" >&2
    exit 64
  fi
fi

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif podman compose version >/dev/null 2>&1; then
  compose=(podman compose)
else
  echo "ratings-db.sh: needs docker compose, or podman with a compose provider" >&2
  exit 1
fi

if [ ! -f .env ]; then
  cp .env.example .env
  echo "ratings-db.sh: created .env from .env.example (local-only values)"
fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a

"${compose[@]}" up -d --wait ratings-db || exit 1
if [ "$native" = 1 ]; then
  cd backend || exit 1
  [ "$restore" = 1 ] && exec go run ./cmd/load -restore "$dir"
  exec go run ./cmd/load "${args[@]}" "$dir"
fi
# The container sees only $dir, so symlinks out of it do not resolve.
[ "$restore" = 1 ] && exec "${compose[@]}" --profile load run --rm --build -v "$dir:/data:ro,z" load -restore /data
exec "${compose[@]}" --profile load run --rm --build -v "$dir:/data:ro,z" load "${args[@]}" /data
