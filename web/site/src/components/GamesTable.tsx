/** A games table, the same wherever games are listed: date, what each game belongs to, where it was played, the players and scores, a replay code to copy, and pages; a blitz edition marks the signed-in player's game. */
import { useState, type ReactNode } from 'react';
import type { GameRow } from '../lib/games-table';
import PlayerLink from './PlayerLink';
import './TournamentSeries.css';
import './GamesTable.css';

const count = (n: number) => n.toLocaleString('en-US');
const score = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(1));

/** GamesTable shows rows under a caption, context naming their second column (Season, Round). waiting, when given, stands in for the rows while they load; empty is said when there are none. */
export default function GamesTable({ caption, context, rows, waiting, empty }: { caption: string; context: string; rows: GameRow[]; waiting?: ReactNode; empty: string }) {
  return (
    <div className="trophy-table-container games-table">
      <table className="trophy-table">
        <caption className="records-intro">{caption}</caption>
        <thead>
          <tr>
            <th className="col-center">Date</th>
            <th>{context}</th>
            <th>Game</th>
            <th>Players &amp; scores</th>
            <th>Replay code</th>
          </tr>
        </thead>
        <tbody>
          {waiting}
          {waiting === undefined && rows.length === 0 && (
            <tr>
              <td colSpan={5} className="cell-muted">
                {empty}
              </td>
            </tr>
          )}
          {rows.map((r) => (
            <tr key={r.key} className={[r.unplayed ? 'game-never-played' : '', r.you === undefined ? '' : 'game-yours'].join(' ').trim() || undefined}>
              <td className="cell-date">{r.date === '' ? <span className="cell-muted">—</span> : r.date}</td>
              <td className="cell-muted">{r.context}</td>
              <td>
                {r.where}
                {r.tag !== '' && <span className="panel-tag game-state">{r.tag}</span>}
                {r.you !== undefined && (
                  <a className="game-you" href="#your-game">
                    {r.you}
                  </a>
                )}
              </td>
              <td>
                <div className="game-seats">
                  {r.seats.map((s, i) => (
                    <span key={`${s.label}-${i}`} className="game-seat">
                      {s.link === null ? (
                        <span className="game-seat-name">{s.label}</span>
                      ) : (
                        <PlayerLink name={s.link}>{s.label}</PlayerLink>
                      )}
                      {s.detail !== '' && <span className="game-outcome"> {s.detail}</span>}
                      {s.score !== null && ` ${score(s.score)}`}
                      {s.note !== '' && <span className="game-outcome"> ({s.note})</span>}
                    </span>
                  ))}
                </div>
              </td>
              <td>{r.code === null ? <span className="cell-muted">—</span> : <ReplayCode code={r.code} />}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** GamesPages moves between a games table's pages, newer and older; nothing when there is only one. */
export function GamesPages({ page, pages, onPage }: { page: number; pages: number; onPage: (page: number) => void }) {
  if (pages <= 1) return null;
  return (
    <div className="games-pages">
      <button type="button" className="btn-antique" disabled={page <= 1} onClick={() => onPage(page - 1)}>
        ‹ Newer
      </button>
      <span>
        Page {count(page)} of {count(pages)}
      </span>
      <button type="button" className="btn-antique" disabled={page >= pages} onClick={() => onPage(page + 1)}>
        Older ›
      </button>
    </div>
  );
}

/** ReplayCode is a replay code that copies itself when clicked. */
function ReplayCode({ code }: { code: string }) {
  const [note, setNote] = useState('');
  const copy = () => {
    const done = (text: string) => {
      setNote(text);
      setTimeout(() => setNote(''), 1500);
    };
    if (!navigator.clipboard) return done('Select to copy');
    navigator.clipboard.writeText(code).then(
      () => done('Copied'),
      () => done('Select to copy'),
    );
  };
  return (
    <button type="button" className="game-code" onClick={copy} title="Copy the replay code" aria-label={`Copy replay code ${code}`}>
      <code>{code}</code>
      <span className="game-code-note" aria-live="polite">
        {note}
      </span>
    </button>
  );
}
