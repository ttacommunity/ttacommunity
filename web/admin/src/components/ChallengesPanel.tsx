/** The Challenges tab: every challenge entry, newest first, a page at a time, with what admins have marked on it. Read-only for now. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import { formatTime, viewerZone } from '@ttac/shared/time';
import type { Load } from '@ttac/shared/load';
import { adminRequest } from '../lib/admin-api';
import { entryFlags, pageCount } from '../lib/challenge-entries';
import { LoadNotice } from './Notice';

type AdminChallengeEntries = components['schemas']['AdminChallengeEntries'];

/** ChallengesPanel loads the page asked for, the first to begin with. */
export default function ChallengesPanel() {
  const [page, setPage] = useState(1);
  const [entries, setEntries] = useState<Load<AdminChallengeEntries>>({ kind: 'loading' });
  useEffect(() => {
    setEntries({ kind: 'loading' });
    void adminRequest<AdminChallengeEntries>(`/api/challenges/entries?page=${page}`, {}).then((r) =>
      setEntries(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
  }, [page]);
  const pages = entries.kind === 'ready' ? pageCount(entries.data.total, entries.data.page_size) : 1;
  const zone = viewerZone();

  return (
    <section className="rating-card admin-card" aria-labelledby="challenge-entries-heading">
      <h2 id="challenge-entries-heading" className="rating-card-heading">
        Challenge entries
      </h2>
      <p className="muted">
        Every domination entered for a challenge, hidden ones included, newest first. Times are in {zone}. Imported with the
        load job (<code>-challenge-entries</code>).
      </p>
      {entries.kind === 'loading' && <p className="muted">Loading…</p>}
      {entries.kind === 'error' && <LoadNotice message={entries.message} />}
      {entries.kind === 'ready' && (
        <>
          <div className="table-responsive">
            <table className="leaderboard-table admin-table">
              <caption className="visually-hidden">Challenge entries, newest first</caption>
              <thead>
                <tr>
                  <th scope="col">Entered</th>
                  <th scope="col">Challenge</th>
                  <th scope="col">Player</th>
                  <th scope="col">Domination</th>
                  <th scope="col">Proof</th>
                  <th scope="col">Flags</th>
                </tr>
              </thead>
              <tbody>
                {entries.data.entries.map((e) => (
                  <tr key={e.id}>
                    <td>{formatTime(e.entered_at, zone)}</td>
                    <td>{e.challenge_name}</td>
                    <th scope="row">
                      {e.player}
                      {e.member === null && <span className="row-detail muted">not linked to a member</span>}
                    </th>
                    <td>{e.domination}</td>
                    <td>
                      {e.proof_url === null ? (
                        <span className="muted">—</span>
                      ) : (
                        <a href={e.proof_url} target="_blank" rel="noopener noreferrer">
                          Proof
                        </a>
                      )}
                    </td>
                    <td>{entryFlags(e).join(' · ')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="note-actions">
            <button type="button" className="btn-antique" disabled={page <= 1} onClick={() => setPage(page - 1)}>
              Newer
            </button>
            <span className="note-status" role="status">
              Page {page} of {pages} · {entries.data.total.toLocaleString()} entries
            </span>
            <button type="button" className="btn-antique" disabled={page >= pages} onClick={() => setPage(page + 1)}>
              Older
            </button>
          </div>
        </>
      )}
    </section>
  );
}
