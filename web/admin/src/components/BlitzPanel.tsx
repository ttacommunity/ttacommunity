/** The Blitz tab: every edition of every series marked Blitz, with how far it has got and its entrant counts, each opening its page. */
import { useEffect, useState } from 'react';
import { formatLabel, reachedLabel } from '@ttac/shared/blitz';
import type { Load } from '@ttac/shared/load';
import { adminRequest, type AdminBlitzEditionRow } from '../lib/admin-api';
import { ADMIN_EDITION_COLUMNS, editionHref } from '../lib/blitz-page';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import { LoadNotice } from './Notice';

export default function BlitzPanel() {
  const [rows, setRows] = useState<Load<AdminBlitzEditionRow[]>>({ kind: 'loading' });
  useEffect(() => {
    void adminRequest<AdminBlitzEditionRow[]>('/api/blitz', {}).then((r) =>
      setRows(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
  }, []);
  return (
    <section className="rating-card admin-card" aria-labelledby="blitz-heading">
      <h2 id="blitz-heading" className="rating-card-heading">
        Blitz tournaments
      </h2>
      <p className="muted">
        Every edition of a tournament with the Blitz setting (Tournaments → Settings). A name opens the edition: its
        players, rounds and results.
      </p>
      {rows.kind === 'loading' && <p className="muted">Loading…</p>}
      {rows.kind === 'error' && <LoadNotice message={rows.message} />}
      {rows.kind === 'ready' && rows.data.length === 0 && <p className="muted">No tournament has the Blitz setting yet.</p>}
      {rows.kind === 'ready' && rows.data.length > 0 && <EditionsTable editions={rows.data} />}
    </section>
  );
}

/** EditionsTable lists the blitz editions, sortable by any column. */
function EditionsTable({ editions }: { editions: AdminBlitzEditionRow[] }) {
  const [rows, sort, sortBy] = useSort(editions, ADMIN_EDITION_COLUMNS);
  return (
    <div className="table-responsive">
      <table className="leaderboard-table admin-table">
        <caption className="visually-hidden">Blitz editions, their format, how far each has got and its entrants</caption>
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
                {r.flagged > 0 && (
                  <span className="count-badge">
                    {r.flagged}
                    <span className="visually-hidden"> flagged</span>
                  </span>
                )}
              </th>
              <td>{r.edition}</td>
              <td>{formatLabel(r.format)}</td>
              <td>{reachedLabel(r.round, r.format, r.champion)}</td>
              <td>{r.entrants}</td>
              <td>{r.dropped}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
