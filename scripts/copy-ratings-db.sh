#!/bin/sh
# Copy a ratings database into an empty one on the same server, with its grants and sequences (so no run id is
# reused): every rating run, only the current one, or none. Runs in the database's container as its superuser.
# Usage: copy-ratings-db.sh SOURCE TARGET TARGET_OWNER all-runs|latest-run|no-runs INPUT_TABLES_FILE
# (the file is load -input-tables' output; every other table but goose's holds a run's results)
set -eu

src=$1 dst=$2 owner=$3 mode=$4 inputs_file=$5
case "$mode" in all-runs | latest-run | no-runs) ;; *) echo "copy-ratings-db.sh: unknown mode $mode" >&2; exit 64 ;; esac
q() { psql -X -q -v ON_ERROR_STOP=1 -U "$PGSU" -At "$@"; }
PGSU=${PGSU:-ratings}
dump=/tmp/$dst.dump list=/tmp/$dst.list
trap 'rm -f "$dump" "$list"' EXIT

inputs_sql=$(sed "s/.*/'&'/" "$inputs_file" | paste -sd, -)
[ -n "$inputs_sql" ] || { echo "copy-ratings-db.sh: $inputs_file lists no input tables" >&2; exit 1; }
# A run table's partitions (one per run) go with it.
run_tables=$(q -d "$src" -c "SELECT string_agg(relname, ' ' ORDER BY relname) FROM pg_class
  WHERE relnamespace = 'public'::regnamespace AND relkind IN ('r', 'p') AND NOT relispartition
    AND relname NOT IN ('goose_db_version', $inputs_sql)")
without=$(q -d "$src" -c "SELECT string_agg(t, ' ') FROM unnest(string_to_array('$run_tables', ' ')) t
  WHERE NOT EXISTS (SELECT FROM information_schema.columns c WHERE c.table_schema = 'public' AND c.table_name = t AND c.column_name = 'run_id')")
[ -z "$without" ] || { echo "copy-ratings-db.sh: not inputs, yet no run_id: $without" >&2; exit 1; }
exclude=""
if [ "$mode" != all-runs ]; then
  for t in $run_tables; do exclude="$exclude --exclude-table-data-and-children=public.$t"; done
fi
run=""
if [ "$mode" = latest-run ]; then
  run=$(q -d "$src" -c "SELECT run_id FROM current_run WHERE ranking_id = community_ranking()")
  [ -n "$run" ] || { echo "copy-ratings-db.sh: $src has no published run" >&2; exit 1; }
fi

# shellcheck disable=SC2086
pg_dump -Fc -U "$PGSU" -d "$src" $exclude -f "$dump"
# The source owner's default privileges cannot be set by the target's owner, who gets its own, nor can it create an
# extension (seed-dev-db.sh --prewarm does, as the superuser).
pg_restore -l "$dump" | grep -v -e "DEFAULT ACL" -e " EXTENSION " > "$list"
pg_restore --exit-on-error --no-owner --role="$owner" -L "$list" --section=pre-data --section=data -U "$PGSU" -d "$dst" "$dump"

if [ -n "$run" ]; then
  for t in $run_tables; do
    q -d "$src" -c "\\copy (SELECT * FROM public.$t WHERE run_id = $run) TO STDOUT" | q -d "$dst" -c "\\copy public.$t FROM STDIN"
    want=$(q -d "$src" -c "SELECT count(*) FROM public.$t WHERE run_id = $run")
    got=$(q -d "$dst" -c "SELECT count(*) FROM public.$t")
    [ "$got" = "$want" ] || { echo "copy-ratings-db.sh: $t has $got rows of run $run, want $want" >&2; exit 1; }
  done
  # The run it was copied from is not in the target.
  q -d "$dst" -c "UPDATE rating_runs SET copied_from = NULL"
fi
pg_restore --exit-on-error --no-owner --role="$owner" -L "$list" --section=post-data -U "$PGSU" -d "$dst" "$dump"

# Partitions of the runs left out are empty; delete_run would never reach them.
q -d "$dst" <<'SQL'
DO $$
DECLARE
    p regclass;
BEGIN
    FOR p IN SELECT c.oid::regclass FROM pg_class c
             WHERE c.relnamespace = 'public'::regnamespace AND c.relkind = 'r' AND c.relispartition
               AND NOT EXISTS (SELECT 1 FROM rating_runs r
                               WHERE pg_get_expr(c.relpartbound, c.oid) = format('FOR VALUES IN (%L)', r.run_id)) LOOP
        EXECUTE format('DROP TABLE %s', p);
    END LOOP;
END
$$;
SQL
