/** DANeo's Highest Single-Match Scores panel, for any tournament: its ten highest winning scores, with the opponents. */
import type { GameRecord } from '@ttac/shared/ratings';
import { cultureScore, opponentsLine, showsWorldChampion } from '../lib/tournament-series';
import PlayerLink from './PlayerLink';

/** GameRecordsPanel shows the games, or waiting while records is null, and nothing for a tournament with none. */
export default function GameRecordsPanel({
  records,
  waiting,
  title,
  intro,
  slug,
}: {
  records: readonly GameRecord[] | null;
  waiting: string;
  title: string;
  intro: string;
  slug: string;
}) {
  if (records !== null && records.length === 0) return null;
  return (
    <div className="panel-card panel-records">
      <h2 className="panel-header panel-header-split">
        <span className="panel-header-title">
          <span>📈</span> {title}
        </span>
        <span className="panel-tag panel-tag-wide">Top Single-Game Scores</span>
      </h2>
      <p className="records-intro">{intro}</p>
      <div className="trophy-table-container records-table">
        <table className="trophy-table">
          <caption className="visually-hidden">The ten highest winning scores</caption>
          <thead>
            <tr>
              <th scope="col" className="col-rank">
                Rank
              </th>
              <th scope="col">Competitor</th>
              <th scope="col" className="col-right">
                Culture Score
              </th>
              <th scope="col">Opponents &amp; Match Scores</th>
              <th scope="col">Division &amp; Match ID</th>
              <th scope="col" className="col-right">
                Match Date
              </th>
            </tr>
          </thead>
          <tbody>
            {records === null ? (
              <tr>
                <td colSpan={6} className="waiting-row">
                  {waiting}
                </td>
              </tr>
            ) : (
              records.map((g, i) => (
                <tr key={`${g.tournament} ${g.date} ${g.name}`}>
                  <td className="cell-rank">#{i + 1}</td>
                  <td>
                    <PlayerLink name={g.name} className="trophy-player" />{' '}
                    {showsWorldChampion(slug, g.world_champion) && <span className="badge badge-wc trophy-mark">WC</span>}
                  </td>
                  <td className="cell-points">{cultureScore(g.score)}</td>
                  <td className="cell-opponents">
                    {g.opponents.length > 0 ? <span className="cell-opponents-list">{opponentsLine(g.opponents)}</span> : '-'}
                  </td>
                  <td className="cell-muted">{g.tournament}</td>
                  <td className="cell-date">{g.date}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
