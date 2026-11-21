/** A games table, the same wherever games are listed: date, what each game belongs to, where it was played, the players and scores, a replay code to copy, and pages; a blitz edition marks the signed-in player's game. */
import type { ReactNode } from 'react';
import { score } from '@ttac/shared/numbers';
import SortHeader, { alignClass, type Align } from '@ttac/shared/SortHeader';
import type { Sort } from '@ttac/shared/sort';
import type { GameRow, GamesColumn } from '../lib/games-table';
import PlayerLink from './PlayerLink';
import './TournamentSeries.css';
import './GamesTable.css';
import CopyCode from '@ttac/shared/CopyCode';
import ScrollTable from './ScrollTable';

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
  const head = (col: GamesColumn, label: ReactNode, align?: Align) =>
    onSort === undefined ? (
      <th className={alignClass(align)}>{label}</th>
    ) : (
      <SortHeader col={col} sort={sort} onSort={onSort} align={align}>
        {label}
      </SortHeader>
    );
  return (
    <ScrollTable label="Games">
      <table className="leaderboard-table trophy-table">
        <caption className="records-intro">{caption}</caption>
        <thead>
          <tr>
            {head('date', 'Date', 'center')}
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
              <td className="cell-date cell-nowrap">{r.date === '' ? <span className="cell-muted">—</span> : r.date}</td>
              <td className="cell-muted cell-nowrap">{r.context}</td>
              <td>
                <span className="cell-nowrap">{r.where}</span>
                {r.tag !== '' && <span className="tag game-state">{r.tag}</span>}
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
              <td>
                <CopyCode code={r.code} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </ScrollTable>
  );
}

