/** Upstream's Hall of Fame series page: the series' text, figures, trophyboard, and best seasons and games. */
import { useEffect, useState, type ReactNode } from 'react';
import RegulationsPanel from './RegulationsPanel';
import TrophyboardPanel from './TrophyboardPanel';
import GameRecordsPanel from './GameRecordsPanel';
import StoriesPanel, { type StoryView } from './StoriesPanel';
import { currentRun, tournamentSeries, type TournamentSeries, type TrophyRow } from '@ttac/shared/ratings';
import { errorText, type Load } from '@ttac/shared/load';
import { catalogueHref } from '../lib/catalogue-view';
import { cardView } from '../lib/tournament-card';
import TournamentHero from './TournamentHero';
import PlayerLink from './PlayerLink';
import { waitingText } from '../lib/waiting';
import { circuitBox, circuitEra, circuitTitle, leaguePoints, seriesRecord, showsWorldChampion, type Circuit, type Major } from '../lib/tournament-series';
import { count } from '@ttac/shared/numbers';


function WorldChampion() {
  return <span className="badge badge-wc trophy-mark">WC</span>;
}

/** TitleMark shows the board's title in the site's title badges; WC only on the World Championship's board. */
function TitleMark({ row, series }: { row: TrophyRow; series: string }) {
  if (series === 'worlds' && (row.title === 'WC' || row.world_champion)) return <WorldChampion />;
  if (row.title === 'WC') return null;
  return row.title ? <span className={`badge badge-${row.title.toLowerCase()} trophy-mark`}>{row.title}</span> : null;
}

/** TournamentSeriesPage loads the current run's results for a circuit, with its card, rules and stories as admin writes them. */
export default function TournamentSeriesPage({ apiUrl, slug }: { apiUrl: string; slug: Circuit }) {
  const [load, setLoad] = useState<Load<TournamentSeries>>({ kind: 'loading' });

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => tournamentSeries(apiUrl, r.run_id, slug))
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: errorText(e) }));
  }, [apiUrl, slug]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = waitingText(load, 'the records') ?? '';
  const card = cardView(data?.card);
  const name = data?.name ?? '';
  useEffect(() => {
    if (data !== null) document.title = circuitTitle(data.name);
  }, [data]);
  const box = data === null ? null : circuitBox(data.members);

  return (
    <>
      <TournamentHero
        card={card}
        title={name}
        subtitle={card.subtitle}
        facts={[
          { label: 'Recorded Matches', value: data ? count(data.games) : '…' },
          { label: 'Seasons / Editions', value: data ? `${data.seasons} Editions` : '…' },
          { label: 'Cadence & Schedule', value: data ? (card.cadence ?? '') : '…', words: true },
          { label: 'Historical Era', value: data ? circuitEra(data.first_game) : '…', words: true },
        ]}
      />

      {data !== null && data.members.length > 0 && (
        <div className="panel-card series-members">
          <h2 className="panel-header">
            <span>{card.icon}</span> Tournaments in the {name}
          </h2>
          <div className="series-members-list">
            {data.members.map((m) => (
              <a key={m.slug} href={catalogueHref(m.slug)} className="btn-antique">
                {m.name} &rarr;
              </a>
            ))}
          </div>
        </div>
      )}
      <SeriesSections
        content={{ slug, name }}
        data={data}
        waiting={waiting}
        regulations={box === null ? null : <RegulationsPanel box={box} rated />}
        stories={data?.stories ?? []}
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
  stories,
}: {
  content: Major;
  data: TournamentSeries | null;
  waiting: string;
  regulations: ReactNode;
  stories: readonly StoryView[];
}) {
  const honours = content.slug === 'grand_slams';
  return (
    <>
      <div className="content-two-col">
        {regulations}

        <TrophyboardPanel
          rows={data?.board ?? null}
          waiting={waiting}
          mark={(b) => <TitleMark row={b} series={content.slug} />}
          honours={honours}
        />
      </div>

      <StoriesPanel stories={stories} />

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
                    <td colSpan={6} className="waiting-row">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.season_records.map((s, i) => (
                    <tr key={`${s.season} ${s.name}`}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <PlayerLink name={s.name} className="trophy-player" /> {showsWorldChampion(content.slug, s.world_champion) && <WorldChampion />}
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

      <GameRecordsPanel
        records={data?.game_records ?? null}
        waiting={waiting}
        title={`Premier Division Highest Single-Match Scores (${content.name})`}
        intro="Historic single-match cultural scoring peaks achieved in premier division competition (e.g. Master 1, Emperor, Championship Playoff Brackets) from official tournament match logs with full opponent matchups."
        slug={content.slug}
      />
    </>
  );
}
