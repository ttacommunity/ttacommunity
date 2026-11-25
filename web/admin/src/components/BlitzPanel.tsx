/** The Blitz tab: every edition of every series marked Blitz, with how far it has got and its entrant counts, each opening its page. */
import { formatLabel, reachedLabel } from '@ttac/shared/blitz';
import type { Load } from '@ttac/shared/load';
import type { AdminBlitzEditionRow } from '../lib/admin-api';
import { editionHref } from '@ttac/shared/blitz';
import { ADMIN_EDITION_COLUMNS } from '../lib/blitz-page';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import { Loading } from './Notice';
import Count from './Count';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';

/** BlitzPanel lists editions, the blitz editions admin loaded once for this tab and its count. */
export default function BlitzPanel({ editions: rows }: { editions: Load<AdminBlitzEditionRow[]> }) {
  return (
    <AdminCard id="blitz-heading" title="Blitz tournaments">
      <p className="muted">
        Every edition of a tournament with the Blitz setting (Tournaments → Settings). A name opens the edition: its
        players, rounds and results.
      </p>
      <Loading load={rows} />
      {rows.kind === 'ready' && rows.data.length === 0 && <p className="muted">No tournament has the Blitz setting yet.</p>}
      {rows.kind === 'ready' && rows.data.length > 0 && <EditionsTable editions={rows.data} />}
    </AdminCard>
  );
}

/** EditionsTable lists the blitz editions, sortable by any column. */
function EditionsTable({ editions }: { editions: AdminBlitzEditionRow[] }) {
  const [rows, sort, sortBy] = useSort(editions, ADMIN_EDITION_COLUMNS);
  return (
    <AdminTable caption="Blitz editions, their format, how far each has got and its entrants">
      <thead>
        <tr>
          <SortHeader col="tournament" sort={sort} onSort={sortBy}>
            Tournament
          </SortHeader>
          <SortHeader col="edition" sort={sort} onSort={sortBy}>
            Edition
          </SortHeader>
          <SortHeader col="format" sort={sort} onSort={sortBy}>
            Format
          </SortHeader>
          <SortHeader col="reached" sort={sort} onSort={sortBy}>
            Reached
          </SortHeader>
          <SortHeader col="players" sort={sort} onSort={sortBy}>
            In
          </SortHeader>
          <SortHeader col="dropped" sort={sort} onSort={sortBy}>
            Dropped
          </SortHeader>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={`${r.tournament}/${r.edition}`}>
            <th scope="row">
              <a href={editionHref(r.tournament, r.edition)}>
                {r.name} {r.edition}
              </a>
              <Count n={r.flagged} what="flagged" />
            </th>
            <td>{r.edition}</td>
            <td>{formatLabel(r.format)}</td>
            <td>{reachedLabel(r.round, r.format, r.champion)}</td>
            <td>{r.entrants}</td>
            <td>{r.dropped}</td>
          </tr>
        ))}
      </tbody>
    </AdminTable>
  );
}
