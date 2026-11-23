/** DANeo's Official Trophyboard & Hall of Fame panel, for any tournament: each medallist's podiums, or a ladder's wins. */
import { useState, type ReactNode } from 'react';
import { boardCells } from '../lib/tournament-series';
import type { BoardEntry } from '../lib/trophyboard';
import PlayerLink from './PlayerLink';
import Message from './Message';
import Panel from './Panel';
import ScrollTable from './ScrollTable';
import ShowAll from './ShowAll';
import { shownRows } from '../lib/show-all';

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
  const [expanded, setExpanded] = useState(false);
  const board = boardCells(rows ?? []);
  const shown = shownRows(rows ?? [], expanded);
  const span = 2 + board.headings.length + (honours ? 1 : 0);
  return (
    <Panel id="trophyboard" icon="🏅" title="Official Trophyboard & Hall of Fame" aside={<span className="tag">{rows !== null ? `${rows.length} Competitors` : '…'}</span>}>
      <div className="trophy-sub">
        <span>Verified podium &amp; title records</span>
      </div>
      <ScrollTable label="Trophyboard">
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
              shown.map((b, i) => (
                <tr key={b.name}>
                  <td className="cell-num">{i + 1}</td>
                  <td>
                    <PlayerLink name={b.name} />{' '}
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
      </ScrollTable>
      <ShowAll total={rows?.length ?? 0} expanded={expanded} onToggle={() => setExpanded(!expanded)} />
    </Panel>
  );
}
