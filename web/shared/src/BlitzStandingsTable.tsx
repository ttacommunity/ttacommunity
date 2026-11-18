/** BlitzStandingsTable is the head and body of a blitz edition's standings, sortable by any column, for the site and admin alike; each puts its own table around it. */
import type { ReactNode } from 'react';
import SortHeader, { useSort } from './SortHeader';
import { STANDING_COLUMNS, drawLabel, gamesCount, gamesNote, pointsNote, resultsLabel, startLabel } from './blitz';
import type { BlitzStanding } from './game-form';

interface Props {
  standings: BlitzStanding[];
  /** What fixed the starting ranks: null before round 1. */
  fixedBy: 'rating' | 'lot' | null;
  tieBreaks: boolean;
  /** What shows before a player's name, such as their flag. */
  marks?: (s: BlitzStanding) => ReactNode;
}

export default function BlitzStandingsTable({ standings, fixedBy, tieBreaks, marks }: Props) {
  const [rows, sort, sortBy] = useSort(standings, STANDING_COLUMNS);
  const start = startLabel(fixedBy);
  return (
    <>
      <thead>
        <tr>
          <SortHeader col="rank" sort={sort} onSort={sortBy} className="num">
            Rank
          </SortHeader>
          <SortHeader col="player" sort={sort} onSort={sortBy}>
            Player
          </SortHeader>
          <SortHeader col="points" sort={sort} onSort={sortBy} className="num">
            Points
          </SortHeader>
          <SortHeader col="played" sort={sort} onSort={sortBy} className="num">
            <span data-tooltip="Games played and forfeits">Games</span>
          </SortHeader>
          <SortHeader col="results" sort={sort} onSort={sortBy} className="num">
            <span data-tooltip="Win–Loss–Tie">Results</span>
          </SortHeader>
          <SortHeader col="byes" sort={sort} onSort={sortBy} className="num">
            Byes
          </SortHeader>
          {tieBreaks && (
            <>
              <SortHeader col="cut1" sort={sort} onSort={sortBy} className="num">
                <abbr title="Buchholz Cut 1">BH-C1</abbr>
              </SortHeader>
              <SortHeader col="buchholz" sort={sort} onSort={sortBy} className="num">
                <abbr title="Buchholz">BH</abbr>
              </SortHeader>
              <SortHeader col="sb" sort={sort} onSort={sortBy} className="num">
                <abbr title="Sonneborn-Berger">SB</abbr>
              </SortHeader>
            </>
          )}
          {start !== null && (
            <SortHeader col="start" sort={sort} onSort={sortBy} className="num">
              {start}
            </SortHeader>
          )}
        </tr>
      </thead>
      <tbody>
        {rows.map((s) => {
          const note = pointsNote(s);
          const gNote = gamesNote(s);
          return (
            <tr key={s.name} className={s.status === 'dropped' ? 'blitz-dropped' : undefined}>
              <td className="num">
                <DrawMark s={s} fixedBy={fixedBy} />
                {s.rank}
              </td>
              <th scope="row">
                {marks?.(s)}
                {s.name}
                {s.status === 'dropped' && <span className="blitz-note">dropped</span>}
              </th>
              <td className="num">
                {note !== null ? (
                  <span data-tooltip={note}>
                    {s.points}
                    <span className="blitz-draw">*</span>
                  </span>
                ) : (
                  s.points
                )}
              </td>
              <td className="num">
                {gNote !== null ? (
                  <span data-tooltip={gNote}>
                    {gamesCount(s)}
                    <span className="blitz-draw">*</span>
                  </span>
                ) : (
                  gamesCount(s)
                )}
              </td>
              <td className="num">{resultsLabel(s)}</td>
              <td className="num">{s.byes}</td>
              {tieBreaks && (
                <>
                  <td className="num">{s.buchholz_cut1}</td>
                  <td className="num">{s.buchholz}</td>
                  <td className="num">{s.sonneborn_berger}</td>
                </>
              )}
              {start !== null && <td className="num">{s.starting_rank ?? '–'}</td>}
            </tr>
          );
        })}
      </tbody>
    </>
  );
}

/** DrawMark marks a rank the lot or the seed ordered, before the number in a slot of its own so the numbers line up, with the ranks it was ordered among as its text; the slot stays empty for a rank held alone or before round 1. */
function DrawMark({ s, fixedBy }: { s: BlitzStanding; fixedBy: 'rating' | 'lot' | null }) {
  const label = drawLabel(s, fixedBy);
  return (
    <span className="blitz-draw-slot">
      {label !== null && (
        <span className="blitz-draw" title={label}>
          *<span className="visually-hidden">{label}</span>
        </span>
      )}
    </span>
  );
}
