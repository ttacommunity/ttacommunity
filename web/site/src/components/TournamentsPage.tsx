/** The Tournaments page: every tournament with games, or one tournament's champions, stages and games. */
import { useEffect, useState } from 'react';
import {
  RatingsApiError,
  catalogue,
  catalogueTournament,
  currentRun,
  tournamentSeries,
  type Catalogue,
  type CatalogueSeason,
  type CatalogueTournament,
  type EditionRules,
  type LadderWin,
  type TournamentSeries,
} from '@ttac/shared/ratings';
import { SERIES_CONTENT, SINGLE_TOURNAMENTS } from '../lib/tournament-series';
import { SeriesSections } from './TournamentSeriesPage';
import { playerHref } from '../lib/player-view';
import TournamentCard from './TournamentCard';
import TournamentGames from './TournamentGames';
import { catalogueHref, cgeYears, championsFromFinals, entryRows, placeLabel, placeNames, seasonName, shownName, spanYears, stageName, tournamentParam, winsLabel } from '../lib/catalogue-view';

type Load<T> = { kind: 'loading' } | { kind: 'missing' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

const count = (n: number) => n.toLocaleString('en-US');

/** TournamentsPage shows the tournament the address names (?t=), or the list of every tournament. */
export default function TournamentsPage({ apiUrl }: { apiUrl: string }) {
  const [slug] = useState(() => tournamentParam(new URLSearchParams(window.location.search)));
  return slug === null ? <TournamentList apiUrl={apiUrl} /> : <TournamentDetail apiUrl={apiUrl} slug={slug} />;
}

/** failed turns a failed load into the page's state: a 404 is a tournament with no games. */
function failed(e: unknown): Load<never> {
  if (e instanceof RatingsApiError && e.status === 404) return { kind: 'missing' };
  return { kind: 'error', message: e instanceof Error ? e.message : String(e) };
}

function Waiting({ load }: { load: Load<unknown> }) {
  if (load.kind === 'error') return <p className="records-intro">Could not load the tournaments: {load.message}</p>;
  if (load.kind === 'missing') {
    return (
      <p className="records-intro">
        No tournament with games has that name. <a href={catalogueHref()}>See every tournament</a>.
      </p>
    );
  }
  return <p className="records-intro">Loading…</p>;
}

function TournamentList({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load<Catalogue>>({ kind: 'loading' });
  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => catalogue(apiUrl, r.run_id))
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl]);
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          <span>📜</span> Every Tournament
        </h1>
        <div className="tournaments-subtitle">
          Every tournament in the record, with its seasons, games and players. The Hall of Fame's series pages hold the
          trophyboards; here is each tournament's history, season by season.
        </div>
      </div>
      {load.kind !== 'ready' ? (
        <Waiting load={load} />
      ) : (
        <div className="series-grid">
          {load.data.tournaments.map((t) => {
            const c = SINGLE_TOURNAMENTS.has(t.slug as never) ? SERIES_CONTENT.find((x) => x.slug === t.slug) : undefined;
            return (
              <TournamentCard
                key={t.slug}
                href={catalogueHref(t.slug)}
                name={shownName(c?.name ?? t.name, t.rankings.length > 0)}
                icon={c?.icon ?? '📜'}
                badge={c?.formatBadge}
                summary={c?.summary}
                rows={entryRows(t)}
                cta={c !== undefined ? 'View Champions, Rules & Games' : 'View Champions & Games'}
              />
            );
          })}
        </div>
      )}
      {load.kind === 'ready' && <CgeList data={load.data} />}
    </>
  );
}

/** CgeList lists every tournament on CGE's site, marking which are ours and rated. */
function CgeList({ data }: { data: Catalogue }) {
  const withGames = new Set(data.tournaments.map((t) => t.slug));
  return (
    <div className="panel-card">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">Tournaments on CGE</h2>
        <span className="panel-tag">{count(data.cge_tournaments.length)} tournaments</span>
      </div>
      <p className="records-intro">
        The tournaments on CGE&apos;s site that count towards the ratings. One marked (Not Rated) is shown but does not
        count.
      </p>
      <div className="trophy-table-container">
        <table className="trophy-table">
          <caption className="records-intro">Tournaments on CGE, newest first</caption>
          <thead>
            <tr>
              <th>Tournament</th>
              <th>Organiser</th>
              <th className="col-center">Years</th>
              <th className="col-center">Status</th>
            </tr>
          </thead>
          <tbody>
            {data.cge_tournaments.map((c) => (
              <tr key={c.cge_tournament}>
                <td>
                  <a href={c.url} rel="noopener noreferrer">
                    {shownName(c.name, c.rated)}
                  </a>
                  {withGames.has(c.tournament) && (
                    <>
                      {' · '}
                      <a href={catalogueHref(c.tournament)}>its seasons</a>
                    </>
                  )}
                </td>
                <td>{c.organiser}</td>
                <td className="cell-date">{cgeYears(c.started, c.finished)}</td>
                <td className="cell-muted">{c.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function TournamentDetail({ apiUrl, slug }: { apiUrl: string; slug: string }) {
  const [load, setLoad] = useState<Load<{ run: number; t: CatalogueTournament }>>({ kind: 'loading' });
  const [pick, setPick] = useState<{ season: string; n: number }>({ season: '', n: 0 });
  const content = SINGLE_TOURNAMENTS.has(slug as never) ? SERIES_CONTENT.find((c) => c.slug === slug) : undefined;
  const [series, setSeries] = useState<{ data: TournamentSeries | null; waiting: string }>({ data: null, waiting: 'Loading…' });
  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => {
        if (content !== undefined) {
          tournamentSeries(apiUrl, r.run_id, content.slug)
            .then((data) => setSeries({ data, waiting: '' }))
            .catch((e: unknown) => setSeries({ data: null, waiting: e instanceof Error ? e.message : 'The records could not be loaded.' }));
        }
        return catalogueTournament(apiUrl, r.run_id, slug).then((t) => ({ run: r.run_id, t }));
      })
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug, content]);
  if (load.kind !== 'ready') return <Waiting load={load} />;
  const { run, t } = load.data;
  const games = t.seasons.reduce((n, s) => n + s.games, 0);
  const staged = t.seasons.filter((s) => s.stages.length > 1 || s.stages[0]?.label !== '');
  const finals = championsFromFinals(t.slug, t.seasons);
  const pickSeason = (season: string) => setPick((p) => ({ season, n: p.n + 1 }));
  return (
    <>
      <div className="detail-hero">
        <div className="detail-hero-top">
          <div className="detail-hero-left">
            <div className="detail-hero-icon">{content?.icon ?? '📜'}</div>
            <div>
              <h1 className="detail-hero-title">{content?.name ?? t.name}</h1>
              <div className="detail-hero-subtitle">
                {content !== undefined && <>{content.subtitle} · </>}
                <a href={catalogueHref()}>Every tournament</a> · <a href="/ratings/hall_of_fame/">Hall of Fame</a>
              </div>
            </div>
          </div>
          {content !== undefined && <div className="detail-hero-badge">{content.formatBadge}</div>}
        </div>
        {content !== undefined && <div className="detail-hero-desc">{content.summary}</div>}
        <div className="detail-meta-grid">
          <div className="meta-box">
            <div className="meta-box-label">Active History</div>
            <div className="meta-box-val">{spanYears(t.seasons[0].first_game, t.seasons[t.seasons.length - 1].last_game)}</div>
          </div>
          <div className="meta-box">
            <div className="meta-box-label">Seasons</div>
            <div className="meta-box-val">{count(t.seasons.length)}</div>
          </div>
          <div className="meta-box">
            <div className="meta-box-label">Recorded Database Matches</div>
            <div className="meta-box-val">{count(games)}</div>
          </div>
          {content !== undefined && (
            <div className="meta-box">
              <div className="meta-box-label">Cadence &amp; Schedule</div>
              <div className="meta-box-val meta-box-text">{content.frequency}</div>
            </div>
          )}
        </div>
      </div>
      {t.ladder && <LadderWinners wins={t.top_tier_wins} />}
      {finals !== null && <Champions seasons={finals} podium title="Grand Prix champions" onPick={pickSeason} />}
      <Champions seasons={t.seasons} podium={!t.ladder && finals === null} title={t.ladder || finals !== null ? 'Seasons' : 'Champions'} onPick={pickSeason} />
      {content !== undefined && <SeriesSections content={content} data={series.data} waiting={series.waiting || 'Loading…'} />}
      {content === undefined && t.rules.length > 0 && <Rules editions={t.rules} />}
      {staged.map((s) => (
        <Stages key={s.label} season={s} />
      ))}
      <TournamentGames apiUrl={apiUrl} run={run} slug={t.slug} seasons={t.seasons} players={t.players} pick={pick} />
    </>
  );
}

/** LadderWinners lists a ladder's top-tier winners, most wins first, as its organisers count them. */
function LadderWinners({ wins }: { wins: LadderWin[] }) {
  return (
    <div className="panel-card champions-panel">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">Top-tier wins</h2>
        <span className="panel-tag">{count(wins.length)} winners</span>
      </div>
      {wins.length === 0 ? (
        <p className="records-intro">The organisers' count of top-tier wins is not recorded yet.</p>
      ) : (
        <>
          <ul className="ladder-winners">
            {wins.map((w) => (
              <li key={w.name}>
                <a href={playerHref(w.name)}>{w.name}</a>
                {winsLabel(w.wins) !== '' && <span className="cell-muted"> · {winsLabel(w.wins)}</span>}
              </li>
            ))}
          </ul>
          <p className="records-intro champions-note">A ladder's champion wins its top tier; a tie counts half. Source: {wins[0].source}.</p>
        </>
      )}
    </div>
  );
}

/** Champions lists seasons, newest first, with their podiums (when podium), figures and CGE pages; a season's name opens its games. */
function Champions({ seasons, podium, title, onPick }: { seasons: CatalogueSeason[]; podium: boolean; title: string; onPick: (season: string) => void }) {
  const place = (s: CatalogueSeason, n: number) => {
    const got = placeNames(s.podium, n);
    if (got.length === 0) return <span className="cell-muted">—</span>;
    return got.map((p, i) => (
      <span key={`${p.name}-${p.final}`} className="champion-name">
        {i > 0 && ' · '}
        <a href={playerHref(p.name)}>{p.name}</a>
        {p.final !== '' && <span className="cell-muted"> ({p.final})</span>}
      </span>
    ));
  };
  return (
    <div className="panel-card champions-panel">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">{title}</h2>
        <span className="panel-tag">{count(seasons.length)} seasons</span>
      </div>
      <div className="trophy-table-container champions-table">
        <table className="trophy-table">
          <caption className="records-intro">
            {podium ? "Each season's top three, newest first; choose a season to see its games" : 'Newest first; choose a season to see its games'}
          </caption>
          <thead>
            <tr>
              <th>Season</th>
              {podium && (
                <>
                  <th>{placeLabel(1)}</th>
                  <th>{placeLabel(2)}</th>
                  <th>{placeLabel(3)}</th>
                </>
              )}
              <th className="col-center">Played</th>
              <th className="col-num">Games</th>
              <th className="col-num">Players</th>
              <th>CGE</th>
            </tr>
          </thead>
          <tbody>
            {[...seasons].reverse().map((s) => (
              <tr key={s.label}>
                <td>
                  <button type="button" className="season-pick" onClick={() => onPick(s.label)} title={`Show the games of ${seasonName(s.label)}`}>
                    {seasonName(s.label)}
                  </button>
                  {s.note !== null && <div className="season-note">{s.note}</div>}
                </td>
                {podium && (
                  <>
                    <td className="champion-cell">{place(s, 1)}</td>
                    <td>{place(s, 2)}</td>
                    <td>{place(s, 3)}</td>
                  </>
                )}
                <td className="cell-date">
                  {s.first_game} – {s.last_game}
                </td>
                <td className="cell-num">{count(s.games)}</td>
                <td className="cell-num">{count(s.players)}</td>
                <td>
                  {s.cge_pages.length === 0 ? (
                    <span className="cell-muted">—</span>
                  ) : (
                    s.cge_pages.map((url, i) => (
                      <a key={url} href={url} target="_blank" rel="noopener noreferrer" className="cge-link">
                        {s.cge_pages.length > 1 ? `CGE ${i + 1}` : 'CGE'} ↗
                      </a>
                    ))
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {podium && (
        <p className="records-intro champions-note">A league's podium is its top division; a cup's is its final. A dash means we have not recorded it yet.</p>
      )}
    </div>
  );
}

/** Rules shows the newest edition's description from CGE, usually its organiser's format and rules, and the earlier ones folded. */
function Rules({ editions }: { editions: EditionRules[] }) {
  const [latest, ...earlier] = editions;
  return (
    <div className="panel-card rules-panel">
      <h2 className="panel-header panel-header-split">
        <span className="panel-header-title">
          <span>📜</span> Format &amp; Regulations
        </span>
        <a href={latest.url} target="_blank" rel="noopener noreferrer" className="cge-link">
          {latest.name} on CGE ↗
        </a>
      </h2>
      <p className="rules-text">{latest.description}</p>
      {earlier.map((e) => (
        <details key={e.cge_tournament} className="rules-earlier">
          <summary>{e.name}</summary>
          <p className="rules-text">{e.description}</p>
          <a href={e.url} target="_blank" rel="noopener noreferrer" className="cge-link">
            {e.name} on CGE ↗
          </a>
        </details>
      ))}
      <p className="records-intro champions-note">As each edition's page on CGE describes it.</p>
    </div>
  );
}

/** Stages lists one season's stages, for a season played in several. */
function Stages({ season: s }: { season: CatalogueSeason }) {
  return (
    <div className="panel-card stages-panel">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">{seasonName(s.label)}: stages</h2>
        <span className="panel-tag">
          {s.first_game} – {s.last_game} · {count(s.games)} games · {count(s.players)} players
        </span>
      </div>
      {s.note !== null && <p className="records-intro">{s.note}</p>}
      <div className="trophy-table-container">
        <table className="trophy-table">
          <caption className="records-intro">Stages of {seasonName(s.label)}</caption>
          <thead>
            <tr>
              <th>Stage</th>
              <th className="col-center">First game</th>
              <th className="col-center">Last game</th>
              <th className="col-num">Games</th>
              <th className="col-num">Players</th>
            </tr>
          </thead>
          <tbody>
            {s.stages.map((st) => (
              <tr key={st.label}>
                <td>{stageName(st.label)}</td>
                <td className="cell-date">{st.first_game}</td>
                <td className="cell-date">{st.last_game}</td>
                <td className="cell-num">{count(st.games)}</td>
                <td className="cell-num">{count(st.players)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
