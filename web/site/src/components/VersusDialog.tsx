/** Upstream's head-to-head popup: two players' latest shared games, filtered by format. */
import { useEffect, useRef, useState } from 'react';
import { playerVersus, type Format, type PlayerVersus } from '@ttac/shared/ratings';
import { versusTally } from '../lib/career';
import { playerHref } from '../lib/player-view';
import Segmented from './Segmented';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: PlayerVersus };

const FORMATS: { value: Format; label: string }[] = [
  { value: 0, label: 'All Formats' },
  { value: 2, label: '2-Player' },
  { value: 3, label: '3-Player' },
  { value: 4, label: '4-Player' },
];

/** VersusDialog shows name's shared games with other, opening at format; onClose runs when it closes. */
export default function VersusDialog({
  apiUrl,
  runId,
  name,
  other,
  format: initial,
  onClose,
}: {
  apiUrl: string;
  runId: number;
  name: string;
  other: string;
  format: Format;
  onClose: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [state, setState] = useState<Load>({ kind: 'loading' });
  const [format, setFormat] = useState<Format>(initial);

  useEffect(() => {
    const d = dialog.current;
    if (d === null) return;
    d.showModal();
    // A click on the backdrop closes it, as upstream's; Esc and × are the keyboard's ways.
    const onBackdrop = (e: MouseEvent) => e.target === d && d.close();
    d.addEventListener('click', onBackdrop);
    let stale = false;
    playerVersus(apiUrl, runId, name, other)
      .then((data) => !stale && setState({ kind: 'ready', data }))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
    return () => {
      stale = true;
      d.removeEventListener('click', onBackdrop);
    };
  }, [apiUrl, runId, name, other]);

  const tally = state.kind === 'ready' ? versusTally(state.data.matches, format) : null;
  const a = state.kind === 'ready' ? state.data.name : name;
  const b = state.kind === 'ready' ? state.data.other : other;

  return (
    <dialog
      ref={dialog}
      className="h2h-modal"
      aria-labelledby="h2h-title"
      onClose={onClose}
    >
      <div className="h2h-head">
        <h3 id="h2h-title">
          {a} vs {b} - Exclusive History
        </h3>
        <button type="button" className="h2h-close" aria-label="Close" onClick={() => dialog.current?.close()}>
          ×
        </button>
      </div>
      <div className="h2h-toolbar">
        <span className="h2h-toolbar-label">Match Format:</span>
        <Segmented label="Match format" look="raised" groupClass="format-bar h2h-formats" options={FORMATS} value={format} onChange={setFormat} />
      </div>
      <div className="h2h-stats">
        {tally === null ? (
          <span className="h2h-muted">Fetching exclusive game records...</span>
        ) : (
          <>
            <div>
              Total Mutual Games: <strong className="h2h-total">{tally.games}</strong>
            </div>
            <div>
              <strong className="h2h-win">
                {a} Wins: {tally.nameWins}
              </strong>
            </div>
            <div>
              <strong className="h2h-loss">
                {b} Wins: {tally.otherWins}
              </strong>
            </div>
            <div>
              <strong className="h2h-draw">Draws / Splits: {tally.draws}</strong>
            </div>
          </>
        )}
      </div>
      <div className="h2h-body">
        {state.kind === 'loading' && <div className="h2h-message">Loading match records...</div>}
        {state.kind === 'error' && <div className="h2h-message h2h-error">Failed to load match history: {state.message}</div>}
        {tally !== null && tally.games === 0 && (
          <div className="h2h-message">No direct tournament encounters found for this player format.</div>
        )}
        {tally !== null && tally.games > 0 && (
          <table className="data-table">
            <caption className="visually-hidden">
              Games {a} and {b} played together
            </caption>
            <thead>
              <tr>
                <th>Date</th>
                <th>Tournament</th>
                <th>Format</th>
                <th>{a} Finish</th>
                <th>{b} Finish</th>
                <th>Table Scores</th>
              </tr>
            </thead>
            <tbody>
              {tally.rows.map((m) => (
                <tr key={m.match_id}>
                  <td>{m.date}</td>
                  <td className="h2h-tournament">{m.tournament || 'Competitive Match'}</td>
                  <td>{m.player_count}p</td>
                  <td>
                    <span className={`placement-badge place-${m.name_place}`}>#{m.name_place}</span>
                  </td>
                  <td>
                    <span className={`placement-badge place-${m.other_place}`}>#{m.other_place}</span>
                  </td>
                  <td className="h2h-scores">
                    {m.seats.map((s, i) => (
                      <span key={`${s.name}-${i}`}>
                        <span className={s.name === a || s.name === b ? 'h2h-target' : 'h2h-other'}>
                          <a href={playerHref(s.name)} className="player-link">
                            {s.name}
                          </a>
                          : {s.score}
                        </span>
                        {i < m.seats.length - 1 ? ', ' : ''}
                      </span>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </dialog>
  );
}
