/** Upstream's Hall of Fame series page: the series' text, figures, trophyboard, and best seasons and games. */
import { useEffect, useState, type ReactNode } from 'react';
import RegulationsPanel from './RegulationsPanel';
import { currentRun, tournamentSeries, type TournamentSeries, type TrophyRow } from '@ttac/shared/ratings';
import { playerHref } from '../lib/player-view';
import { catalogueHref } from '../lib/catalogue-view';
import { circuitCard } from '../lib/tournament-card';
import TournamentHero from './TournamentHero';
import {
  cultureScore,
  leaguePoints,
  boardCells,
  opponentsLine,
  seriesEra,
  seriesRecord,
  showsWorldChampion,
  type SeriesContent,
} from '../lib/tournament-series';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: TournamentSeries };

const count = (n: number) => n.toLocaleString('en-US');

function Player({ name }: { name: string }) {
  return (
    <a href={playerHref(name)} className="trophy-player">
      {name}
    </a>
  );
}

function WorldChampion() {
  return <span className="badge badge-wc trophy-mark">WC</span>;
}

/** TitleMark shows the board's title in the site's title badges; WC only on the World Championship's board. */
function TitleMark({ row, series }: { row: TrophyRow; series: string }) {
  if (series === 'worlds' && (row.title === 'WC' || row.world_champion)) return <WorldChampion />;
  if (row.title === 'WC') return null;
  return row.title ? <span className={`badge badge-${row.title.toLowerCase()} trophy-mark`}>{row.title}</span> : null;
}

/** TournamentSeriesPage loads the current run's results for a series and shows them as upstream's page does. */
export default function TournamentSeriesPage({ apiUrl, content }: { apiUrl: string; content: SeriesContent }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => tournamentSeries(apiUrl, r.run_id, content.slug))
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl, content.slug]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = load.kind === 'error' ? load.message : 'Loading…';

  return (
    <>
      <TournamentHero
        card={circuitCard(content)}
        title={content.name}
        subtitle={content.subtitle}
        facts={[
          { label: 'Recorded Matches', value: data ? count(data.games) : '…' },
          { label: 'Seasons / Editions', value: data ? `${data.seasons} Editions` : '…' },
          { label: 'Cadence & Schedule', value: content.frequency, words: true },
          { label: 'Historical Era', value: content.era ?? (data ? seriesEra(content, data.seasons) : '…'), words: true },
        ]}
      />

      {content.members !== undefined && (
        <div className="panel-card series-members">
          <h2 className="panel-header">
            <span>{content.icon}</span> Tournaments in the {content.name}
          </h2>
          <div className="series-members-list">
            {content.members.map((m) => (
              <a key={m.slug} href={catalogueHref(m.slug)} className="btn-antique">
                {m.name} &rarr;
              </a>
            ))}
          </div>
        </div>
      )}
      <SeriesSections
        content={content}
        data={data}
        waiting={waiting}
        regulations={<RegulationsPanel box={{ lines: content.rules ?? [], source: null }} rated />}
      />
    </>
  );
}

/** SeriesSections shows a series' rules, trophyboard, stories and best seasons and games, as upstream's series page does; data null is loading, with waiting shown. */
export function SeriesSections({
  content,
  data,
  waiting,
  regulations,
}: {
  content: SeriesContent;
  data: TournamentSeries | null;
  waiting: string;
  regulations: ReactNode;
}) {
  const board = boardCells(data?.board ?? []);
  const honours = content.slug === 'grand_slams';
  return (
    <>
      <div className="content-two-col">
        {regulations}

        <div className="panel-card">
          <h2 className="panel-header panel-header-split">
            <span className="panel-header-title">
              <span>🏅</span> Official Trophyboard &amp; Hall of Fame
            </span>
            <span className="panel-tag">{data ? `${data.board.length} Competitors` : '…'}</span>
          </h2>
          <div className="trophy-sub">
            <span>Verified podium &amp; title records</span>
            <span className="trophy-sub-hint">Scroll table ↕</span>
          </div>
          <div className="trophy-table-container">
            <table className="trophy-table">
              <caption className="visually-hidden">Each medallist's podiums in the top division</caption>
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
                {data === null ? (
                  <tr>
                    <td colSpan={2 + board.headings.length + (honours ? 1 : 0)} className="series-message">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.board.map((b, i) => {
                    return (
                      <tr key={b.name}>
                        <td className="cell-num">{i + 1}</td>
                        <td>
                          <Player name={b.name} /> <TitleMark row={b} series={content.slug} />
                        </td>
                        {board.rows[i]?.map((cell, j) => (
                          <td key={board.headings[j]} className={`col-center medal-count ${board.classes[j] ?? ''}`}>
                            {cell}
                          </td>
                        ))}
                        {honours && <td className="cell-honours">{b.details}</td>}
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {content.stories.length > 0 && (
        <div className="panel-card panel-stories">
          <h2 className="panel-header">
            <span>⚡</span> Master+ Dynastic Milestones &amp; Champion Chronicles
          </h2>
          <div className="stories-grid">
            {content.stories.map((s) => (
              <div key={s.player} className="story-card">
                <div className="story-header">
                  <a href={playerHref(s.player)} className="story-author">
                    <span>👤</span> {s.player}
                  </a>
                  <div className="story-badge">{s.badge}</div>
                </div>
                <div className="story-title">{s.title}</div>
                <div className="story-text">{s.text}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {(data === null || data.season_records.length > 0) && (
        <div className="panel-card panel-records">
          <h2 className="panel-header panel-header-split">
            <span className="panel-header-title">
              <span>🏆</span> Premier Division All-Time Seasonal Records ({content.name})
            </span>
            <span className="panel-tag panel-tag-wide">Highest Seasonal League Points</span>
          </h2>
          <p className="records-intro">
            All-time highest cumulative league points achieved in a single premier division season (4P: 6/3/1/0 pts; 3P: 5/2/0
            pts; 2P: 2/1/0 pts; ties split points equally). 9 Seasons have been played prior to the CGE official tournament page
            tracking, so to find the coressponding season there you need to subtract 9.
          </p>
          <div className="trophy-table-container records-table">
            <table className="trophy-table">
              <caption className="visually-hidden">The ten highest-scoring top-division seasons</caption>
              <thead>
                <tr>
                  <th scope="col" className="col-rank">
                    Rank
                  </th>
                  <th scope="col">Competitor</th>
                  <th scope="col" className="col-right">
                    League Points
                  </th>
                  <th scope="col" className="col-center">
                    Record
                  </th>
                  <th scope="col">Season &amp; Division</th>
                  <th scope="col" className="col-right">
                    Season Date
                  </th>
                </tr>
              </thead>
              <tbody>
                {data === null ? (
                  <tr>
                    <td colSpan={6} className="series-message">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.season_records.map((s, i) => (
                    <tr key={`${s.season} ${s.name}`}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <Player name={s.name} /> {showsWorldChampion(content.slug, s.world_champion) && <WorldChampion />}
                      </td>
                      <td className="cell-points">{leaguePoints(s.points)}</td>
                      <td className="cell-record">
                        <span className="cell-record-value">{seriesRecord(content.slug, s.placings)}</span>
                      </td>
                      <td className="cell-muted">
                        Season {s.season} ({s.division})
                      </td>
                      <td className="cell-date">{s.last_played}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {(data === null || data.game_records.length > 0) && (
        <div className="panel-card panel-records">
          <h2 className="panel-header panel-header-split">
            <span className="panel-header-title">
              <span>📈</span> Premier Division Highest Single-Match Scores ({content.name})
            </span>
            <span className="panel-tag panel-tag-wide">Top Single-Game Scores</span>
          </h2>
          <p className="records-intro">
            Historic single-match cultural scoring peaks achieved in premier division competition (e.g. Master 1, Emperor,
            Championship Playoff Brackets) from official tournament match logs with full opponent matchups.
          </p>
          <div className="trophy-table-container records-table">
            <table className="trophy-table">
              <caption className="visually-hidden">The ten highest winning scores in top-division games</caption>
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
                {data === null ? (
                  <tr>
                    <td colSpan={6} className="series-message">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.game_records.map((g, i) => (
                    <tr key={`${g.tournament} ${g.date} ${g.name}`}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <Player name={g.name} /> {showsWorldChampion(content.slug, g.world_champion) && <WorldChampion />}
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
      )}
    </>
  );
}
