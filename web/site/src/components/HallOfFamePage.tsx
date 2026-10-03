/** Upstream's Hall of Fame overview: the World Champions, each series' card, and the all-time, cups and ladders tables. */
import { useEffect, useState } from 'react';
import { catalogue, currentRun, hallOfFame, type CatalogueEntry, type HallOfFame } from '@ttac/shared/ratings';
import TournamentCard from './TournamentCard';
import { playerHref } from '../lib/player-view';
import { catalogueHref, entryRows } from '../lib/catalogue-view';
import {
  derivedMatches,
  legendMark,
  majorMedals,
  majorTitles,
  SERIES_CONTENT,
  SINGLE_TOURNAMENTS,
  seriesEra,
  seriesGames,
  seriesHref,
  titleCell,
} from '../lib/tournament-series';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: HallOfFame };

// Upstream's overview states the champions and their honours in its template.
const CHAMPIONS = [
  {
    name: 'a440',
    icon: '👑',
    year: 2025,
    text: '2025 Reigning World Champion • Stage 4 Upper Playoff Champion • Highest Elo/WHR ceiling in modern 3P & 4P finals.',
  },
  {
    name: 'Martin_Pecheur',
    icon: '⚔️',
    year: 2024,
    text: '2024 World Champion • 3x Intermezzo Champion • 2x Royal League Champion • All-round multi-format master.',
  },
  {
    name: 'Weidenbaum',
    icon: '🏛️',
    year: 2023,
    text: 'Inaugural 2023 World Champion • 6x International Champion • 8x Intermezzo Champion • All-time peak titleholder.',
  },
];

/** HallOfFamePage loads the current run's overview and shows it as upstream's page does. */
export default function HallOfFamePage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });

  const [tournaments, setTournaments] = useState<CatalogueEntry[]>([]);

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => {
        catalogue(apiUrl, r.run_id)
          .then((c) => setTournaments(c.tournaments))
          .catch(() => setTournaments([]));
        return hallOfFame(apiUrl, r.run_id);
      })
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = load.kind === 'error' ? load.message : 'Loading…';
  const count = (slug: string) => data?.series.find((s) => s.series === slug);

  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          <span>🏛️</span> Official Through the Ages Hall of Fame
        </h1>
        <div className="tournaments-subtitle">
          The definitive historical registry of competitive Through the Ages achievements. Explore verified trophyboards,
          matchdata-derived season champions, World Championship playoff bracket analytics, and career milestones of Master+
          competitors.
        </div>
        <div className="tournaments-stats-bar">
          <div className="stat-pill">
            Series Active: <strong>6 Major Circuits</strong>
          </div>
          <div className="stat-pill">
            Derived Matches: <strong>{data ? derivedMatches(data.series) : '…'}</strong>
          </div>
          <div className="stat-pill">
            Historical Span: <strong>2016 – Present</strong>
          </div>
          <div className="stat-pill">
            World Champions: <strong>3 Crowned Titleholders</strong>
          </div>
        </div>
      </div>

      <div className="podium-preview-card">
        <h2 className="podium-preview-title">
          <span>👑</span> World Champions &amp; Dynastic Benchmark Legends
        </h2>
        <p className="podium-intro">
          The official annual World Championship crowns the definitive global Through the Ages champion. Staged across
          multi-round qualification groups and playoff brackets (with evolving structural formats across editions).
        </p>
        <div className="champions-highlight-grid">
          {CHAMPIONS.map((c) => (
            <div key={c.name} className="champ-highlight-box">
              <div className="champ-highlight-avatar">{c.icon}</div>
              <div className="champ-highlight-info">
                <a href={playerHref(c.name)} className="champ-highlight-name">
                  {c.name} <span className="badge-wc">👑 {c.year} World Champion</span>
                </a>
                <div className="champ-highlight-desc">{c.text}</div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="series-grid">
        {SERIES_CONTENT.map((s) => {
          const entry = SINGLE_TOURNAMENTS.has(s.slug) ? tournaments.find((t) => t.slug === s.slug) : undefined;
          const c = count(s.slug);
          const rows: [string, string][] = SINGLE_TOURNAMENTS.has(s.slug)
            ? entry !== undefined
              ? entryRows(entry)
              : [['Recorded Database Matches', '…']]
            : [
                ['Structure & Cadence', s.frequency],
                ['Active History', s.era ?? (c ? seriesEra(s, c.seasons) : '…')],
                ['Recorded Database Matches', c ? seriesGames(c) : '…'],
              ];
          return (
            <TournamentCard
              key={s.slug}
              href={seriesHref(s.slug)}
              name={s.name}
              icon={s.icon}
              badge={s.formatBadge}
              summary={s.summary}
              rows={rows}
              cta={SINGLE_TOURNAMENTS.has(s.slug) ? 'View Champions, Rules & Games' : 'View Board & Tournaments'}
            />
          );
        })}
        <TournamentCard
          href={catalogueHref()}
          name="Every Tournament"
          icon="📜"
          summary="Every tournament in the record, beyond the six circuits: its champions, seasons, stages and games."
          rows={[]}
          cta="View Every Tournament"
        />
      </div>

      <div className="wcs-analytics-card">
        <div className="hof-card-head">
          <div>
            <h2 className="hof-card-title hof-card-title-large">👑 All-Time Major Circuits Hall of Fame</h2>
            <div className="hof-card-sub hof-card-sub-large">
              Verified Premier Division titles and podium medals won strictly in the 4 official major circuits: World
              Championship, International Championship (3/4P), Intermezzo Championship (3P), and Royal League (2P).
            </div>
          </div>
          <span className="hof-tag">Major League Titles Only</span>
        </div>
        <div className="scroll-hint">
          <span>Swipe table horizontally for full records ➔</span>
        </div>
        <div className="table-responsive">
          <table className="wcs-table">
            <caption className="visually-hidden">Titles and podium medals across the four major circuits</caption>
            <thead>
              <tr>
                <th scope="col" className="col-rank-wide">
                  Rank
                </th>
                <th scope="col">Competitor</th>
                <th scope="col" className="col-center">
                  Major Titles 🏆
                </th>
                <th scope="col" className="col-center">
                  World (WCS) 👑
                </th>
                <th scope="col" className="col-center">
                  International (4P) 🌐
                </th>
                <th scope="col" className="col-center">
                  Intermezzo (3P) ⚔️
                </th>
                <th scope="col" className="col-center">
                  Royal League (2P) 👑
                </th>
                <th scope="col" className="col-right">
                  Major Medals (🥇 / 🥈 / 🥉)
                </th>
              </tr>
            </thead>
            <tbody>
              {data === null ? (
                <tr>
                  <td colSpan={8} className="hof-message">
                    {waiting}
                  </td>
                </tr>
              ) : (
                data.legends.map((l, i) => {
                  const mark = legendMark(l);
                  return (
                    <tr key={l.name}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <a href={playerHref(l.name)} className="legend-player">
                          {l.name}
                        </a>{' '}
                        {mark === 'WC' && <span className="badge-wc legend-mark">👑 WC</span>}
                        {mark === 'GM' && <span className="legend-mark">GM</span>}
                      </td>
                      <td className="cell-titles">{majorTitles(l)}</td>
                      <td className="cell-world">{titleCell(l.world_titles, true)}</td>
                      <td className="cell-international">{titleCell(l.international_titles)}</td>
                      <td className="cell-intermezzo">{titleCell(l.intermezzo_titles)}</td>
                      <td className="cell-royal">{titleCell(l.royal_league_titles)}</td>
                      <td className="cell-medals">{majorMedals(l)}</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="hof-two-col">
        <div className="wcs-analytics-card">
          <div className="hof-card-head">
            <div>
              <h2 className="hof-card-title">
                <span>🎾</span> All-Time Grand Slams &amp; Cups Hall of Fame
              </h2>
              <div className="hof-card-sub">Survivors Cup, Wimbledon TTA, Eiffel Tower, Slow Burn &amp; Major Opens</div>
            </div>
          </div>
          <div className="scroll-hint">
            <span>Swipe table horizontally for full records ➔</span>
          </div>
          <div className="table-responsive">
            <table className="wcs-table">
              <caption className="visually-hidden">The cups&apos; leading medallists</caption>
              <thead>
                <tr>
                  <th scope="col" className="col-rank-narrow">
                    Rank
                  </th>
                  <th scope="col">Competitor</th>
                  <th scope="col" className="col-center">
                    🥇 Gold
                  </th>
                  <th scope="col" className="col-center">
                    🥈 Silver
                  </th>
                  <th scope="col">Notable Cup Titles</th>
                </tr>
              </thead>
              <tbody>
                {data === null ? (
                  <tr>
                    <td colSpan={5} className="hof-message">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.grand_slams.map((g, i) => (
                    <tr key={g.name}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <a href={playerHref(g.name)} className="board-player">
                          {g.name}
                        </a>
                      </td>
                      <td className="cell-gold">{g.gold}</td>
                      <td className="cell-silver">{g.silver}</td>
                      <td className="cell-cups">{g.details}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div className="wcs-analytics-card">
          <div className="hof-card-head">
            <div>
              <h2 className="hof-card-title">
                <span>🪜</span> All-Time Competitive Ladders Hall of Fame
              </h2>
              <div className="hof-card-sub">
                Top competitors on the Mercurial, Sodium, and Transcontinental (TCL) challenge ladders
              </div>
            </div>
          </div>
          <div className="scroll-hint">
            <span>Swipe table horizontally for full records ➔</span>
          </div>
          <div className="table-responsive">
            <table className="wcs-table">
              <caption className="visually-hidden">The ladders&apos; most winning players</caption>
              <thead>
                <tr>
                  <th scope="col" className="col-rank-narrow">
                    Rank
                  </th>
                  <th scope="col">Competitor</th>
                  <th scope="col" className="col-right">
                    Ladder Wins
                  </th>
                  <th scope="col" className="col-right">
                    Matches
                  </th>
                  <th scope="col" className="col-center">
                    Win Rate
                  </th>
                </tr>
              </thead>
              <tbody>
                {data === null ? (
                  <tr>
                    <td colSpan={5} className="hof-message">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.ladders.map((l, i) => (
                    <tr key={l.name}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <a href={playerHref(l.name)} className="board-player">
                          {l.name}
                        </a>
                      </td>
                      <td className="cell-wins">{l.wins} W</td>
                      <td className="cell-games">{l.games}</td>
                      <td className="cell-rate">{l.win_rate?.toFixed(1)}%</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </>
  );
}
