/** The Tournaments page: every tournament with games, or one tournament's champions, stages and games. */
import { useEffect, useState } from 'react';
import {
  RatingsApiError,
  catalogue,
  catalogueTournament,
  currentRun,
  type Catalogue,
  type CatalogueSeason,
  type CatalogueTournament,
} from '@ttac/shared/ratings';
import { playerHref } from '../lib/player-view';
import TournamentGames from './TournamentGames';
import { catalogueHref, cgeYears, placeLabel, placeNames, seasonName, shownName, spanYears, stageName, tournamentParam } from '../lib/catalogue-view';

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
          {load.data.tournaments.map((t) => (
            <a key={t.slug} href={catalogueHref(t.slug)} className="series-card">
              <div>
                <div className="series-name">{shownName(t.name, t.rankings.length > 0)}</div>
                <div className="series-meta-row">
                  <span>Active History</span>
                  <span>{spanYears(t.first_game, t.last_game)}</span>
                </div>
                <div className="series-meta-row">
                  <span>Seasons</span>
                  <span>{count(t.seasons)}</span>
                </div>
                <div className="series-meta-row">
                  <span>Recorded Database Matches</span>
                  <span>{count(t.games)}</span>
                </div>
                <div className="series-meta-row">
                  <span>Players</span>
                  <span>{count(t.players)}</span>
                </div>
              </div>
              <div className="series-cta">
                <span>View Seasons &amp; Podiums</span>
                <span className="arrow">&rarr;</span>
              </div>
            </a>
          ))}
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
  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => catalogueTournament(apiUrl, r.run_id, slug).then((t) => ({ run: r.run_id, t })))
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug]);
  if (load.kind !== 'ready') return <Waiting load={load} />;
  const { run, t } = load.data;
  const games = t.seasons.reduce((n, s) => n + s.games, 0);
  const staged = t.seasons.filter((s) => s.stages.length > 1 || s.stages[0]?.label !== '');
  return (
    <>
      <div className="detail-hero">
        <div className="detail-hero-top">
          <div className="detail-hero-left">
            <div className="detail-hero-icon">📜</div>
            <div>
              <h1 className="detail-hero-title">{t.name}</h1>
              <div className="detail-hero-subtitle">
                <a href={catalogueHref()}>Every tournament</a>
              </div>
            </div>
          </div>
        </div>
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
        </div>
      </div>
      <Champions seasons={t.seasons} onPick={(season) => setPick((p) => ({ season, n: p.n + 1 }))} />
      {staged.map((s) => (
        <Stages key={s.label} season={s} />
      ))}
      <TournamentGames apiUrl={apiUrl} run={run} slug={t.slug} seasons={t.seasons} pick={pick} />
    </>
  );
}

/** Champions lists every season, newest first, with its podium, figures and CGE pages; a season's name opens its games. */
function Champions({ seasons, onPick }: { seasons: CatalogueSeason[]; onPick: (season: string) => void }) {
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
        <h2 className="panel-header-title">Champions</h2>
        <span className="panel-tag">{count(seasons.length)} seasons</span>
      </div>
      <div className="trophy-table-container champions-table">
        <table className="trophy-table">
          <caption className="records-intro">Each season's top three, newest first; choose a season to see its games</caption>
          <thead>
            <tr>
              <th>Season</th>
              <th>{placeLabel(1)}</th>
              <th>{placeLabel(2)}</th>
              <th>{placeLabel(3)}</th>
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
                </td>
                <td className="champion-cell">{place(s, 1)}</td>
                <td>{place(s, 2)}</td>
                <td>{place(s, 3)}</td>
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
      <p className="records-intro champions-note">
        A league's podium is its top division; a cup's is its final. A dash means we have not recorded it yet.
      </p>
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
