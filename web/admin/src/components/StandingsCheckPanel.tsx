/** CGE's standings tables checked against our games: the totals, and each player who differs. */
import { useState } from 'react';
import { adminRequest } from '../lib/admin-api';
import { refusalOf, type Outcome } from '../lib/outcome';
import Notice from './Notice';
import { DEFAULT_KINDS, checkSummary, differenceRows, type Kind, type StandingsCheck } from '../lib/standings-check';

const count = (n: number) => n.toLocaleString();

/** StandingsCheckPanel runs the check when asked (it reads every game), then lists the differences of the kinds ticked. */
export default function StandingsCheckPanel() {
  const [check, setCheck] = useState<StandingsCheck | null>(null);
  const [status, setStatus] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const [shown, setShown] = useState<ReadonlySet<Kind>>(DEFAULT_KINDS);
  const run = async () => {
    setStatus('Checking every stage…');
    setRefusal(null);
    const r = await adminRequest<StandingsCheck>('/api/standings-check', {});
    setStatus('');
    setRefusal(refusalOf(r));
    if (r.ok) setCheck(r.data);
  };
  const toggle = (kind: Kind) => {
    const next = new Set(shown);
    if (!next.delete(kind)) next.add(kind);
    setShown(next);
  };
  const summary = check === null ? null : checkSummary(check);
  const rows = check === null ? [] : differenceRows(check, shown);

  return (
    <section className="rating-card admin-card" aria-labelledby="standings-check-heading">
      <h2 id="standings-check-heading" className="rating-card-heading">
        Standings check
      </h2>
      <p className="muted">
        Each player's games and places in every CGE stage we hold, against CGE's own standings table for it. Loaded with the
        load job (<code>-facts cge_standings</code>).
      </p>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void run()}>
          {check === null ? 'Run the check' : 'Run it again'}
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
      {summary !== null && (
        <>
          <p>
            {count(summary.agree)} of {count(summary.players)} players agree ({summary.share}) in {count(summary.stages)} stages.{' '}
            <span className="muted">{count(summary.onlyOnCGE)} stages are on CGE only.</span>
          </p>
          <fieldset className="check-kinds">
            <legend>Show</legend>
            {summary.byKind.map((k) => (
              <label key={k.kind}>
                <input type="checkbox" checked={shown.has(k.kind)} onChange={() => toggle(k.kind)} /> {k.label} ({count(k.count)})
              </label>
            ))}
          </fieldset>
          {rows.length === 0 ? (
            <p className="muted">No differences of the kinds shown.</p>
          ) : (
            <div className="table-responsive">
              <table className="leaderboard-table admin-table">
                <caption className="visually-hidden">Players whose games or places differ from CGE's table</caption>
                <thead>
                  <tr>
                    <th scope="col">Stage</th>
                    <th scope="col">Player</th>
                    <th scope="col">Difference</th>
                    <th scope="col">CGE</th>
                    <th scope="col">Here</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.key}>
                      <td>{r.stage}</td>
                      <td>{r.player}</td>
                      <td>{r.kind}</td>
                      <td>{r.cge}</td>
                      <td>{r.held}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </section>
  );
}
