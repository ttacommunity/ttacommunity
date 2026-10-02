/** The Tournaments page: every tournament with games, or one tournament's seasons, stages and podiums. */
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
import { catalogueHref, cgeYears, placeLabel, seasonName, shownName, spanYears, stageName, tournamentParam } from '../lib/catalogue-view';

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
  const [load, setLoad] = useState<Load<CatalogueTournament>>({ kind: 'loading' });
  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => catalogueTournament(apiUrl, r.run_id, slug))
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug]);
  if (load.kind !== 'ready') return <Waiting load={load} />;
  const t = load.data;
  const games = t.seasons.reduce((n, s) => n + s.games, 0);
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
      {t.seasons.map((s) => (
        <Season key={s.label} season={s} />
      ))}
    </>
  );
}

function Season({ season: s }: { season: CatalogueSeason }) {
  const staged = s.stages.length > 1 || s.stages[0]?.label !== '';
  return (
    <div className="panel-card">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">{seasonName(s.label)}</h2>
        <span className="panel-tag">
          {s.first_game} – {s.last_game} · {count(s.games)} games · {count(s.players)} players
        </span>
      </div>
      {s.podium.length > 0 && (
        <ul className="rules-list">
          {s.podium.map((p) => (
            <li key={`${p.final}|${p.place}`}>
              {placeLabel(p.place)}: <a href={playerHref(p.name)}>{p.name}</a>
              {p.final !== '' && ` (${p.final})`}
            </li>
          ))}
        </ul>
      )}
      {staged && (
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
      )}
    </div>
  );
}
