/** DANeo's Official Trophyboard & Hall of Fame panel, for any tournament: each medallist's podiums, or a ladder's wins. */
import type { ReactNode } from 'react';
import { boardCells } from '../lib/tournament-series';
import type { BoardEntry } from '../lib/trophyboard';
import PlayerLink from './PlayerLink';

/** TrophyboardPanel shows the board, or waiting while rows is null; mark adds a player's title, honours their titles column. */
export default function TrophyboardPanel<R extends BoardEntry & { details?: string | null }>({
  rows,
  waiting,
  mark,
  honours = false,
}: {
  rows: readonly R[] | null;
  waiting: string;
  mark?: (row: R) => ReactNode;
  honours?: boolean;
}) {
  const board = boardCells(rows ?? []);
  const span = 2 + board.headings.length + (honours ? 1 : 0);
  return (
    <div className="panel-card">
      <h2 className="panel-header panel-header-split">
        <span className="panel-header-title">
          <span>🏅</span> Official Trophyboard &amp; Hall of Fame
        </span>
        <span className="panel-tag">{rows !== null ? `${rows.length} Competitors` : '…'}</span>
      </h2>
      <div className="trophy-sub">
        <span>Verified podium &amp; title records</span>
        <span className="trophy-sub-hint">Scroll table ↕</span>
      </div>
      <div className="trophy-table-container">
        <table className="trophy-table">
          <caption className="visually-hidden">Each medallist's podiums</caption>
          <thead>
            <tr>
              <th scope="col" className="col-num">
                #
              </th>
              <th scope="col">Competitor</th>
              {board.headings.map((h) => (
                <th key={h} scope="col" className="col-center">
                  {h}
                </th>
              ))}
              {honours && <th scope="col">Honors &amp; Titles</th>}
            </tr>
          </thead>
          <tbody>
            {rows === null || rows.length === 0 ? (
              <tr>
                <td colSpan={span} className="series-message">
                  {rows === null ? waiting : 'No podiums recorded yet.'}
                </td>
              </tr>
            ) : (
              rows.map((b, i) => (
                <tr key={b.name}>
                  <td className="cell-num">{i + 1}</td>
                  <td>
                    <PlayerLink name={b.name} className="trophy-player" />{' '}
                    {mark?.(b)}
                  </td>
                  {board.rows[i]?.map((cell, j) => (
                    <td key={board.headings[j]} className={`col-center medal-count ${board.classes[j] ?? ''}`}>
                      {cell}
                    </td>
                  ))}
                  {honours && <td className="cell-honours">{b.details}</td>}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
