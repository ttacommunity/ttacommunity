/** The Show all button on /design, under a sample table of invented rows, as every long table has it. */
import { useState } from 'react';
import ScrollTable from './ScrollTable';
import ShowAll from './ShowAll';
import { shownRows } from '../lib/show-all';

const ROWS = Array.from({ length: 24 }, (_, i) => ({ rank: i + 1, name: `Sample Player ${i + 1}`, titles: 24 - i }));

/** ShowAllSample shows the first rows of 24, and all of them once its button is pressed. */
export default function ShowAllSample() {
  const [expanded, setExpanded] = useState(false);
  return (
    <>
      <ScrollTable label="Sample long table">
        <table className="leaderboard-table">
          <caption className="visually-hidden">A sample long table</caption>
          <thead>
            <tr>
              <th scope="col">#</th>
              <th scope="col">Player</th>
              <th scope="col" className="num">
                Titles
              </th>
            </tr>
          </thead>
          <tbody>
            {shownRows(ROWS, expanded).map((r) => (
              <tr key={r.rank}>
                <td>{r.rank}</td>
                <td>{r.name}</td>
                <td className="num">{r.titles}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollTable>
      <ShowAll total={ROWS.length} expanded={expanded} onToggle={() => setExpanded(!expanded)} />
    </>
  );
}
