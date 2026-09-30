#!/usr/bin/env bash
# Prints a count and checksum of each table a rating run writes, so a run on one database can be compared with a
# run on another: equal lines mean equal results. Usage: scripts/run-checksums.sh <database-url> <run-id>
set -euo pipefail

url=${1:?usage: run-checksums.sh <database-url> <run-id>}
run=${2:?usage: run-checksums.sh <database-url> <run-id>}
[[ $run =~ ^[0-9]+$ ]] || { echo "run-checksums: run id must be a number" >&2; exit 2; }

tables=$(psql -X -At "$url" -c "
  SELECT table_name FROM information_schema.columns
  WHERE table_schema = 'public' AND column_name = 'run_id' AND table_name NOT IN ('rating_runs', 'current_run')
  ORDER BY table_name")

for t in $tables; do
  # Every column but the run id and serial ids, which differ between databases.
  cols=$(psql -X -At "$url" -c "
    SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position) FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = '$t' AND column_name NOT IN ('run_id', 'pairwise_id', 'id')")
  psql -X -At "$url" -c "
    SELECT '$t', count(*), md5(coalesce(string_agg(r::text, E'\\n' ORDER BY r::text), ''))
    FROM (SELECT $cols FROM $t WHERE run_id = $run) r"
done
