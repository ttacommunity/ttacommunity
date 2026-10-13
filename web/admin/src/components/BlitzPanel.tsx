/** The Blitz tab: every edition of every series marked Blitz, with how far it has got and its entrant counts, each opening its page. */
import { useEffect, useState } from 'react';
import { formatLabel, reachedLabel } from '@ttac/shared/blitz';
import { adminRequest, type AdminBlitzEditionRow, type Load } from '../lib/admin-api';
import { editionHref } from '../lib/blitz-page';

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
      {rows.kind === 'error' && (
        <p className="row-error" role="alert">
          {rows.message}
        </p>
      )}
      {rows.kind === 'ready' && rows.data.length === 0 && <p className="muted">No tournament has the Blitz setting yet.</p>}
      {rows.kind === 'ready' && rows.data.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Blitz editions, their format, how far each has got and its entrants</caption>
            <thead>
              <tr>
                <th scope="col">Tournament</th>
                <th scope="col">Edition</th>
                <th scope="col">Format</th>
                <th scope="col">Reached</th>
                <th scope="col">In</th>
                <th scope="col">Pending</th>
                <th scope="col">Dropped</th>
              </tr>
            </thead>
            <tbody>
              {rows.data.map((r) => (
                <tr key={`${r.tournament}/${r.edition}`}>
                  <th scope="row">
                    <a href={editionHref(r.tournament, r.edition)}>
                      {r.name} {r.edition}
                    </a>
                  </th>
                  <td>{r.edition}</td>
                  <td>{formatLabel(r.format)}</td>
                  <td>{reachedLabel(r.round, r.format, r.champion)}</td>
                  <td>{r.entrants}</td>
                  <td>{r.pending}</td>
                  <td>{r.dropped}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
