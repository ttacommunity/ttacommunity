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
  type TournamentSeries,
} from '@ttac/shared/ratings';
import { MAJORS, majorName } from '../lib/tournament-series';
import { SeriesSections } from './TournamentSeriesPage';
import TournamentCard from './TournamentCard';
import TournamentHero from './TournamentHero';
import RegulationsPanel from './RegulationsPanel';
import TrophyboardPanel from './TrophyboardPanel';
import GameRecordsPanel from './GameRecordsPanel';
import StoriesPanel from './StoriesPanel';
import { ladderBoard, podiumBoard } from '../lib/trophyboard';
import { cardView } from '../lib/tournament-card';
import TournamentGames from './TournamentGames';
import { activeYears, catalogueHref, cgeYears, entryRows, hasPodiums, placeLabel, placeNames, plural, regulationsBox, seasonDates, seasonName, shownName, stageName, tournamentParam } from '../lib/catalogue-view';
import PlayerLink from './PlayerLink';
import Waiting from './Waiting';
import { waitingText } from '../lib/waiting';

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

function Missing() {
  return (
    <div className="rating-card waiting-message">
      No tournament with games has that name. <a href={catalogueHref()}>See every tournament</a>.
    </div>
  );
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
        load.kind === 'missing' ? <Missing /> : <Waiting load={load} what="the tournaments" />
      ) : (
        <div className="series-grid">
          {load.data.tournaments.map((t) => {
            const major = majorName(t.slug);
            return (
              <TournamentCard
                key={t.slug}
                href={catalogueHref(t.slug)}
                name={shownName(major ?? t.name, t.rankings.length > 0)}
                card={cardView(t.card)}
                rows={entryRows(t)}
                cta={major !== undefined ? 'View Champions, Rules & Games' : 'View Champions & Games'}
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
  const content = MAJORS.find((m) => m.slug === slug);
  const [series, setSeries] = useState<{ data: TournamentSeries | null; waiting: string }>({ data: null, waiting: waitingText({ kind: 'loading' }, 'the records') ?? '' });
  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => {
        if (content !== undefined) {
          tournamentSeries(apiUrl, r.run_id, content.slug)
            .then((data) => setSeries({ data, waiting: '' }))
            .catch((e: unknown) => setSeries({ data: null, waiting: waitingText({ kind: 'error', message: e instanceof Error ? e.message : String(e) }, 'the records') ?? '' }));
        }
        return catalogueTournament(apiUrl, r.run_id, slug).then((t) => ({ run: r.run_id, t }));
      })
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug, content]);
  if (load.kind === 'missing') return <Missing />;
  if (load.kind !== 'ready') return <Waiting load={load} what="this tournament" />;
  const { run, t } = load.data;
  const games = t.seasons.reduce((n, s) => n + s.games, 0);
  const staged = t.seasons.filter((s) => s.stages.length > 1 || s.stages[0]?.label !== '');
  const pickSeason = (season: string) => setPick((p) => ({ season, n: p.n + 1 }));
  const box = regulationsBox(t);
  const regulations = box === null ? null : <RegulationsPanel box={box} rated={t.rated} />;
  const card = cardView(t.card);
  return (
    <>
      <TournamentHero
        card={card}
        title={content?.name ?? t.name}
        subtitle={
          <>
            {card.subtitle !== null && <>{card.subtitle} · </>}
            <a href={catalogueHref()}>Every tournament</a> · <a href="/ratings/hall_of_fame/">Hall of Fame</a>
          </>
        }
        facts={[
          { label: 'Active History', value: activeYears(t.seasons) },
          { label: plural(t.season_name), value: count(t.seasons.length) },
          { label: 'Recorded Database Matches', value: count(games) },
          ...(card.cadence !== null ? [{ label: 'Cadence & Schedule', value: card.cadence, words: true }] : []),
        ]}
      />
      <Champions seasons={t.seasons} podium={hasPodiums(t.seasons)} onPick={pickSeason} word={t.season_name} />
      {content !== undefined && (
        <SeriesSections
          content={content}
          data={series.data}
          waiting={series.waiting}
          regulations={regulations}
          stories={t.stories}
        />
      )}
      {content === undefined && (
        <div className="content-two-col">
          {regulations}
          <TrophyboardPanel rows={t.ladder ? ladderBoard(t.top_tier_wins) : podiumBoard(t.seasons)} waiting="Loading the records…" />
        </div>
      )}
      {content === undefined && <StoriesPanel stories={t.stories} />}
      {content === undefined && (
        <GameRecordsPanel
          records={t.game_records}
          waiting="Loading the records…"
          title={`Highest Single-Match Scores (${t.name})`}
          intro="Historic single-match cultural scoring peaks achieved in this tournament, from its official match logs with full opponent matchups."
          slug={t.slug}
        />
      )}
      <Stages seasons={staged} seasonWord={t.season_name} stageWord={t.stage_name} />
      <TournamentGames apiUrl={apiUrl} run={run} slug={t.slug} seasons={t.seasons} players={t.players} pick={pick} seasonWord={t.season_name} stageWord={t.stage_name} />
    </>
  );
}

/** Champions lists seasons (in the tournament's word), newest first, with their podiums (when podium), figures and CGE pages; a season's name opens its games. */
function Champions({
  seasons,
  podium,
  onPick,
  word,
}: {
  seasons: CatalogueSeason[];
  podium: boolean;
  onPick: (season: string) => void;
  word: string;
}) {
  const lower = word.toLowerCase();
  const place = (s: CatalogueSeason, n: number) => {
    const got = placeNames(s.podium, n);
    if (got.length === 0) return <span className="cell-muted">—</span>;
    return got.map((p, i) => (
      <span key={`${p.name}-${p.final}`} className="champion-name">
        {i > 0 && ' · '}
        <PlayerLink name={p.name} />
        {p.final !== '' && <span className="cell-muted"> ({p.final})</span>}
      </span>
    ));
  };
  return (
    <div className="panel-card champions-panel">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">Champions &amp; {plural(word)}</h2>
        <span className="panel-tag">
          {count(seasons.length)} {plural(lower)}
        </span>
      </div>
      <div className="trophy-table-container champions-table">
        <table className="trophy-table">
          <caption className="records-intro">
            {podium ? `Each ${lower}'s top three, newest first; choose a ${lower} to see its games` : `Newest first; choose a ${lower} to see its games`}
          </caption>
          <thead>
            <tr>
              <th>{word}</th>
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
                  <button type="button" className="season-pick" onClick={() => onPick(s.label)} title={`Show the games of ${seasonName(s.label, word)}`}>
                    {seasonName(s.label, word)}
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
                <td className="cell-date">{seasonDates(s)}</td>
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

/** Stages lists the stages of the seasons played in several, one season at a time, the latest first shown, in the tournament's words. */
function Stages({ seasons, seasonWord, stageWord }: { seasons: CatalogueSeason[]; seasonWord: string; stageWord: string }) {
  const [label, setLabel] = useState(seasons[seasons.length - 1]?.label ?? '');
  const s = seasons.find((x) => x.label === label) ?? seasons[seasons.length - 1];
  if (s === undefined) return null;
  return (
    <div className="panel-card stages-panel">
      <div className="panel-header panel-header-split">
        <h2 className="panel-header-title">{plural(stageWord)}</h2>
        {seasons.length > 1 ? (
          <label className="stages-pick">
            <span className="visually-hidden">{seasonWord}</span>
            <select id="stages-season" value={s.label} onChange={(e) => setLabel(e.target.value)}>
              {[...seasons].reverse().map((x) => (
                <option key={x.label} value={x.label}>
                  {seasonName(x.label, seasonWord)}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <span className="panel-tag">{seasonName(s.label, seasonWord)}</span>
        )}
      </div>
      <p className="records-intro">
        {seasonName(s.label, seasonWord)}: {seasonDates(s)} · {count(s.games)} games · {count(s.players)} players
      </p>
      {s.note !== null && <p className="records-intro">{s.note}</p>}
      <div className="trophy-table-container">
        <table className="trophy-table">
          <caption className="records-intro">
            {plural(stageWord)} of {seasonName(s.label, seasonWord)}
          </caption>
          <thead>
            <tr>
              <th>{stageWord}</th>
              <th className="col-center">First game</th>
              <th className="col-center">Last game</th>
              <th className="col-num">Games</th>
              <th className="col-num">Players</th>
            </tr>
          </thead>
          <tbody>
            {s.stages.map((st) => (
              <tr key={st.label}>
                <td>
                  {stageName(st.label)}
                  {st.note !== null && <div className="season-note">{st.note}</div>}
                </td>
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
