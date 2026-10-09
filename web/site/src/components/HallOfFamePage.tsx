/** Upstream's Hall of Fame overview: the World Champions, each series' card, and the all-time, cups and ladders tables. */
import { useEffect, useState } from 'react';
import { catalogue, catalogueTournament, currentRun, hallOfFame, type CatalogueEntry, type CatalogueSeason, type HallOfFame } from '@ttac/shared/ratings';
import TournamentCard from './TournamentCard';
import { cardView, circuitCard } from '../lib/tournament-card';
import { catalogueHref, entryRows } from '../lib/catalogue-view';
import PlayerLink from './PlayerLink';
import {
  derivedMatches,
  legendMark,
  majorMedals,
  majorTitles,
  CIRCUITS,
  MAJORS,
  seriesEra,
  seriesGames,
  seriesHref,
  titleCell,
  worldChampions,
} from '../lib/tournament-series';
import ScrollTable from './ScrollTable';
import { waitingText } from '../lib/waiting';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: HallOfFame };

/** HallOfFamePage loads the current run's overview and shows it as upstream's page does. */
export default function HallOfFamePage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });

  const [tournaments, setTournaments] = useState<CatalogueEntry[]>([]);
  const [finals, setFinals] = useState<CatalogueSeason[]>([]);

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => {
        catalogue(apiUrl, r.run_id)
          .then((c) => setTournaments(c.tournaments))
          .catch(() => setTournaments([]));
        catalogueTournament(apiUrl, r.run_id, 'worlds')
          .then((t) => setFinals(t.seasons))
          .catch(() => setFinals([]));
        return hallOfFame(apiUrl, r.run_id);
      })
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = waitingText(load, 'the Hall of Fame') ?? '';
  const count = (slug: string) => data?.series.find((s) => s.series === slug);
  const champions = worldChampions(finals, data?.legends ?? []);

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
            World Champions: <strong>{new Set(champions.map((c) => c.name)).size} Crowned Titleholders</strong>
          </div>
        </div>
      </div>

      <div className="podium-preview-card">
        <h2 className="podium-preview-title">
          <span>👑</span> World Championship Hall of Fame &amp; Annual Podiums
        </h2>
        <p className="podium-intro">
          The official annual World Championship crowns the definitive global Through the Ages champion. Below are the verified
          podiums (Gold 🥇, Silver 🥈, Bronze 🥉) across each contested edition.
        </p>
        <div className="champions-highlight-grid">
          {finals.filter((s) => s.in_champions && s.podium.length > 0).map((s) => {
            const gold = s.podium.find((p) => p.place === 1);
            const silver = s.podium.find((p) => p.place === 2);
            const bronze = s.podium.find((p) => p.place === 3);
            return (
              <div key={s.label} className="champ-highlight-box wcs-podium-card">
                <div className="champ-highlight-avatar">🏆</div>
                <div className="champ-highlight-info">
                  <div className="wcs-edition-title">World Championship {s.label}</div>
                  <div className="wcs-podium-places">
                    {gold && (
                      <div className="wcs-place-row">
                        <span className="badge badge-wc">WC</span>
                        <PlayerLink name={gold.name} className="champ-highlight-name" />
                      </div>
                    )}
                    {silver && (
                      <div className="wcs-place-row">
                        <span className="badge badge-wcs">WCS</span>
                        <PlayerLink name={silver.name} className="podium-runner-up" />
                      </div>
                    )}
                    {bronze && (
                      <div className="wcs-place-row">
                        <span className="badge badge-wcb">WCB</span>
                        <PlayerLink name={bronze.name} className="podium-third" />
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="series-grid">
        {MAJORS.map((m) => {
          const entry = tournaments.find((t) => t.slug === m.slug);
          return (
            <TournamentCard
              key={m.slug}
              href={seriesHref(m.slug)}
              name={m.name}
              card={cardView(entry?.card)}
              rows={entry !== undefined ? entryRows(entry) : [['Recorded Database Matches', '…']]}
              cta="View Champions, Rules & Games"
            />
          );
        })}
        {CIRCUITS.map((s) => {
          const c = count(s.slug);
          return (
            <TournamentCard
              key={s.slug}
              href={seriesHref(s.slug)}
              name={s.name}
              card={circuitCard(s)}
              rows={[
                ['Structure & Cadence', s.frequency],
                ['Active History', s.era ?? (c ? seriesEra(s, c.seasons) : '…')],
                ['Recorded Database Matches', c ? seriesGames(c) : '…'],
              ]}
              cta="View Board & Tournaments"
            />
          );
        })}
        <TournamentCard
          href={catalogueHref()}
          name="Every Tournament"
          card={cardView({ icon: '📜', tagline: '', subtitle: '', cadence: '', summary: 'Every tournament in the record, beyond the six circuits: its champions, seasons, stages and games.' })}
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
        <ScrollTable hint="full records">
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
                  <td colSpan={8} className="waiting-row">
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
                        <PlayerLink name={l.name} className="legend-player" />{' '}
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
        </ScrollTable>
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
          <ScrollTable hint="full records">
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
                    <td colSpan={5} className="waiting-row">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.grand_slams.map((g, i) => (
                    <tr key={g.name}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <PlayerLink name={g.name} className="board-player" />
                      </td>
                      <td className="cell-gold">{g.gold}</td>
                      <td className="cell-silver">{g.silver}</td>
                      <td className="cell-cups">{g.details}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </ScrollTable>
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
          <ScrollTable hint="full records">
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
                    <td colSpan={5} className="waiting-row">
                      {waiting}
                    </td>
                  </tr>
                ) : (
                  data.ladders.map((l, i) => (
                    <tr key={l.name}>
                      <td className="cell-rank">#{i + 1}</td>
                      <td>
                        <PlayerLink name={l.name} className="board-player" />
                      </td>
                      <td className="cell-wins">{l.wins} W</td>
                      <td className="cell-games">{l.games}</td>
                      <td className="cell-rate">{l.win_rate?.toFixed(1)}%</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </ScrollTable>
        </div>
      </div>
    </>
  );
}
