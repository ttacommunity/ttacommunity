/** DANeo's Official Trophyboard & Hall of Fame panel, for any tournament: each medallist's podiums, or a ladder's wins. */
import type { ReactNode } from 'react';
import { boardCells } from '../lib/tournament-series';
import type { BoardEntry } from '../lib/trophyboard';
import PlayerLink from './PlayerLink';
import Message from './Message';
import Panel from './Panel';

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
    <Panel id="trophyboard" icon="🏅" title="Official Trophyboard & Hall of Fame" aside={<span className="panel-tag">{rows !== null ? `${rows.length} Competitors` : '…'}</span>}>
      <div className="trophy-sub">
        <span>Verified podium &amp; title records</span>
        <span className="trophy-sub-hint">Scroll table ↕</span>
      </div>
      <div className="trophy-table-container">
        <table className="leaderboard-table trophy-table">
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
              <Message form="row" colSpan={span}>
                {rows === null ? waiting : 'No podiums recorded yet.'}
              </Message>
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
    </Panel>
  );
}
