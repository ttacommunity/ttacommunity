/** A games table, the same wherever games are listed: date, what each game belongs to, where it was played, the players and scores, a replay code to copy, and pages; a blitz edition marks the signed-in player's game. */
import type { ReactNode } from 'react';
import { useCopy } from '@ttac/shared/copy';
import { count, score } from '@ttac/shared/numbers';
import SortHeader from '@ttac/shared/SortHeader';
import type { Sort } from '@ttac/shared/sort';
import type { GameRow, GamesColumn } from '../lib/games-table';
import PlayerLink from './PlayerLink';
import './TournamentSeries.css';
import './GamesTable.css';

/** GamesTable shows rows under a caption, context naming their second column (Season, Round). waiting, when given, stands in for the rows while they load; empty is said when there are none. With onSort, its headers sort by their column (the caller orders the rows by sort). */
export default function GamesTable({
  caption,
  context,
  rows,
  waiting,
  empty,
  sort = null,
  onSort,
}: {
  caption: string;
  context: string;
  rows: GameRow[];
  waiting?: ReactNode;
  empty: string;
  sort?: Sort<GamesColumn>;
  onSort?: (col: GamesColumn) => void;
}) {
  const head = (col: GamesColumn, label: ReactNode, className?: string) =>
    onSort === undefined ? (
      <th className={className}>{label}</th>
    ) : (
      <SortHeader col={col} sort={sort} onSort={onSort} className={className}>
        {label}
      </SortHeader>
    );
  return (
    <div className="trophy-table-container games-table">
      <table className="trophy-table">
        <caption className="records-intro">{caption}</caption>
        <thead>
          <tr>
            {head('date', 'Date', 'col-center')}
            {head('context', context)}
            {head('game', 'Game')}
            {head('players', <>Players &amp; scores</>)}
            {head('code', 'Replay code')}
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
  const { copied, copy } = useCopy();
  return (
    <button type="button" className="game-code" onClick={() => copy(code)} title="Copy the replay code" aria-label={`Copy replay code ${code}`}>
      <code>{code}</code>
      <span className="game-code-note" aria-live="polite">
        {copied === null ? '' : copied ? 'Copied' : 'Select to copy'}
      </span>
    </button>
  );
}
