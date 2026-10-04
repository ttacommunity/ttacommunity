/** A player's page for the current run, as upstream's: ratings by model, history chart, games and rivals. */
import { useCallback, useEffect, useState, type MouseEvent, type ReactNode } from 'react';
import {
  currentRun,
  player as fetchPlayer,
  playerAchievements,
  playerCareer,
  playerHistory,
  playerMatches,
  playerRivals,
  type Format,
  type Player,
  type PlayerAchievements,
  type PlayerCareer,
  type PlayerHistory,
  type PlayerMatches,
  type PlayerRivals,
} from '@ttac/shared/ratings';
import CareerBox from './CareerBox';
import { ExtraBadges, Flag } from './PlayerMarks';
import RatingChart from './RatingChart';
import VersusDialog from './VersusDialog';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW, type ResetMode } from '../lib/leaderboard-view';
import {
  firstSort,
  isProvisional,
  matrixHref,
  pageModel,
  parsePlayerView,
  playerParams,
  formatCards,
  score,
  signedDelta,
  visibleRivals,
  RIVALS_SHOWN,
  type PlayerView,
  type RivalKey,
  type RivalsView,
} from '../lib/player-view';
import { achievementsHref } from '../lib/achievements';
import { recall, remember } from '../lib/remembered';
import './PlayerPage.css';
import Segmented from './Segmented';
import FilterInput from './FilterInput';
import PlayerLink from './PlayerLink';
import ScrollTable from './ScrollTable';

type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

const FORMATS: Format[] = [0, 2, 3, 4];
const RESET_MODES: { value: ResetMode; name: string; desc: string }[] = [
  {
    value: 'continuous',
    name: 'Continuous (Career)',
    desc: 'Standard uninterrupted career rating trajectory across all tournament history',
  },
  { value: 'softer', name: 'Season Reset', desc: 'Gentle annual reset balancing career achievement with current season form' },
];

function readView(): PlayerView {
  return parsePlayerView(new URLSearchParams(window.location.search), recall(window.localStorage));
}

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** useLoad runs load whenever key, which names everything load reads, changes; a null load waits. */
function useLoad<T>(key: string, load: (() => Promise<T>) | null): Load<T> {
  const [state, setState] = useState<Load<T>>({ kind: 'loading' });
  useEffect(() => {
    if (load === null) return;
    let stale = false;
    setState({ kind: 'loading' });
    load()
      .then((data) => !stale && setState({ kind: 'ready', data }))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: errorText(e) }));
    return () => {
      stale = true;
    };
  }, [key, load === null]);
  return state;
}

/** Shows the player named in the address. Every choice is part of the address, so a view can be shared. */
export default function PlayerPage({ apiUrl }: { apiUrl: string }) {
  const [view, setView] = useState<PlayerView>(readView);
  const [run, setRun] = useState<Load<number>>({ kind: 'loading' });

  const go = useCallback((next: PlayerView) => {
    window.history.pushState(null, '', `?${playerParams(next)}`);
    remember(window.localStorage, { ...DEFAULT_VIEW, engine: next.engine, reset: next.reset, format: next.format });
    setView(next);
  }, []);
  const linkTo = (next: PlayerView) => ({
    href: `?${playerParams(next)}`,
    onClick: (e: MouseEvent) => {
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      e.preventDefault();
      go(next);
    },
  });

  useEffect(() => {
    const onPop = () => setView(readView());
    window.addEventListener('popstate', onPop);
    currentRun(apiUrl)
      .then((r) => setRun({ kind: 'ready', data: r.run_id }))
      .catch((e: unknown) => setRun({ kind: 'error', message: errorText(e) }));
    return () => window.removeEventListener('popstate', onPop);
  }, [apiUrl]);

  const runId = run.kind === 'ready' && view.name !== '' ? run.data : null;
  const model = pageModel(view.engine, view.reset);
  const { name, format, matchPage } = view;
  const profile = useLoad<Player>(`${runId}|${name}`, runId === null ? null : () => fetchPlayer(apiUrl, runId, name));
  const history = useLoad<PlayerHistory>(
    `${runId}|${name}|${format}`,
    runId === null ? null : () => playerHistory(apiUrl, runId, name, format),
  );
  const games = useLoad<PlayerMatches>(
    `${runId}|${name}|${format}|${model}|${matchPage}`,
    runId === null ? null : () => playerMatches(apiUrl, runId, name, format, model, matchPage),
  );
  const rivals = useLoad<PlayerRivals>(
    `${runId}|${name}|${format}`,
    runId === null ? null : () => playerRivals(apiUrl, runId, name, format),
  );
  const career = useLoad<PlayerCareer>(
    `${runId}|${name}|${format}`,
    runId === null ? null : () => playerCareer(apiUrl, runId, name, format),
  );
  const achievements = useLoad<PlayerAchievements>(
    `${runId}|${name}`,
    runId === null ? null : () => playerAchievements(apiUrl, runId, name),
  );

  useEffect(() => {
    document.title = `${view.name || 'Player'} - Player Profile | Through the Ages`;
  }, [view.name]);

  if (view.name === '') return <Message>No player was named. Choose one from the leaderboard.</Message>;
  if (run.kind === 'error') return <Message>{run.message}</Message>;
  if (profile.kind === 'error') return <Message>{profile.message}</Message>;
  if (profile.kind === 'loading') return <Message>Loading player…</Message>;

  const p = profile.data;
  const provisional = isProvisional(p, view.format);
  const gamesInFormat = p.games.find((g) => g.format === view.format)?.games ?? 0;

  return (
    <>
      <Header player={p} view={view} provisional={provisional} gamesInFormat={gamesInFormat} />

      <div className="profile-selectors">
        <Segmented
          label="Reset mode"
          look="raised"
          groupClass="format-bar profile-bar"
          prefix={<span className="profile-bar-label">Reset Mode:</span>}
          options={RESET_MODES.map((m) => ({ value: m.value, label: m.name, tooltip: m.desc }))}
          value={view.reset}
          link={(reset) => linkTo({ ...view, reset, matchPage: 1 })}
        />
        <Segmented
          label="Format"
          look="raised"
          groupClass="format-bar profile-bar"
          prefix={<span className="profile-bar-label">Format:</span>}
          options={FORMATS.map((f) => ({ value: f, label: show.formatName(f), tooltip: `View rating and history for ${show.formatName(f)}` }))}
          value={view.format}
          link={(format) => linkTo({ ...view, format, matchPage: 1 })}
        />
      </div>

      <CareerBox
        player={p}
        career={career.kind === 'ready' ? career.data : null}
        summary={achievements.kind === 'ready' ? achievements.data.summary : null}
        format={view.format}
      />

      <FormatCards player={p} view={view} linkTo={linkTo} />

      <RatingChart history={history.kind === 'ready' ? history.data : null} />

      <div className="tables-grid">
        <Games state={games} view={view} provisional={provisional} gamesInFormat={gamesInFormat} linkTo={linkTo} go={go} />
        <Rivals state={rivals} compare={runId === null ? null : { apiUrl, runId, format: view.format }} />
      </div>
    </>
  );
}

function Message({ children }: { children: ReactNode }) {
  return <div className="rating-card profile-message">{children}</div>;
}

function Header({
  player,
  view,
  provisional,
  gamesInFormat,
}: {
  player: Player;
  view: PlayerView;
  provisional: boolean;
  gamesInFormat: number;
}) {
  const t = player.title;
  const board = new URLSearchParams({ model: view.engine, format: String(view.format) });
  return (
    <div className="profile-header">
      <div className="player-identity">
        <div className="player-identity-name">
          {t !== null && (
            <span
              className={`badge badge-${t.code.toLowerCase()}${t.code === 'WC' || t.code === 'SGM' ? ' badge-profile-lg' : ''}`}
              data-tooltip={t.reason || show.titleName(t.code)}
            >
              {show.titleName(t.code)}
            </span>
          )}
          <ExtraBadges badges={player.badges} tooltip />
          <h1 className="player-profile-name">
            {player.name}
            {provisional && (
              <>
                {' '}
                <span
                  className="profile-calibrating"
                  data-tooltip={`Provisional / Calibrating Prior: ${gamesInFormat}/15 matches completed`}
                >
                  ⏳
                </span>
              </>
            )}
          </h1>
        </div>
        <div className="player-meta-pill">
          <Flag country={player.country} />
          {player.peak !== null && (
            <>
              <span className="meta-separator">•</span>
              <span>
                Peak Rank:{' '}
                <span className={`badge badge-inline badge-${player.peak.code.toLowerCase()}`}>
                  {show.titleName(player.peak.code)} ({player.peak.year ?? 'Career'})
                </span>
              </span>
            </>
          )}
        </div>
      </div>
      <div className="profile-links">
        <a href={achievementsHref(player.name)} className="btn-tournament-achievements">
          <span aria-hidden="true">🏆</span> Tournament Achievements »
        </a>
        <a href={matrixHref(player.name, view.format)} className="btn-tournament-achievements btn-matrix">
          <span aria-hidden="true">📊</span> Comparison Matrix &amp; Career Years »
        </a>
        <a href={`/ratings/?${board}`} className="home-circle-btn" data-tooltip="Return to Competitive Leaderboard">
          <img src="/img/home-button-circle.webp" alt="Leaderboard" className="home-btn-img" width={58} height={58} />
        </a>
      </div>
    </div>
  );
}

/** FormatCards shows upstream's four GlickoD cards, one per format, each a link to that format; the chosen one is gold. */
function FormatCards({ player, view, linkTo }: { player: Player; view: PlayerView; linkTo: LinkTo }) {
  return (
    <div className="cards-grid">
      {formatCards(player, view.reset).map((c) => {
        const active = c.format === view.format;
        const r = c.rating;
        const classes = ['model-card', 'format-card', active && 'active-card card-daneo'].filter(Boolean).join(' ');
        return (
          <a key={c.format} className={classes} {...linkTo({ ...view, engine: 'glicko2_daneo', format: c.format, matchPage: 1 })}>
            <h3>
              <span>
                {c.icon} {c.label}
              </span>
              <span className="badge badge-rank">{r ? `#${r.rank}` : 'Unranked'}</span>
            </h3>
            <div className="rating-number">
              {r ? (
                <>
                  {r.rating.toFixed(1)} <span className="rd-tag">±{r.rd.toFixed(1)}</span>
                </>
              ) : (
                <span className="format-card-none">No matches</span>
              )}
            </div>
            <div className="c-rating-box">
              <span>Conservative (C = μ - 3σ):</span>
              <strong>{r ? r.c_rating.toFixed(2) : '—'}</strong>
            </div>
            <div className="model-card-caption">
              {r && r.opponents > 0 ? `Matches: ${r.opponents} | Win%: ${r.win_rate.toFixed(1)}%` : c.description}
            </div>
          </a>
        );
      })}
    </div>
  );
}

type LinkTo = (next: PlayerView) => { href: string; onClick: (e: MouseEvent) => void };

function Games({
  state,
  view,
  provisional,
  gamesInFormat,
  linkTo,
  go,
}: {
  state: Load<PlayerMatches>;
  view: PlayerView;
  provisional: boolean;
  gamesInFormat: number;
  linkTo: LinkTo;
  go: (next: PlayerView) => void;
}) {
  const data = state.kind === 'ready' ? state.data : null;
  const total = data?.total ?? gamesInFormat;
  return (
    <div className="rating-card">
      <div className="panel-head">
        <h3>
          Game History <span className="panel-head-count">(Recent Matches: {total})</span>
        </h3>
        {provisional && (
          <span
            className="badge badge-calibration"
            title="Provisional Phase: Player ratings undergo prior calibration during their first 15 matches. Match deltas are displayed normally while the rating converges."
          >
            ⏳ Prior Calibration ({total}/15)
          </span>
        )}
      </div>
      <ScrollTable hint="full match details">
        <table className="data-table">
          <caption className="visually-hidden">
            Games of {view.name}, {show.formatName(view.format)}, page {data?.page ?? view.matchPage}
          </caption>
          <thead>
            <tr>
              <th>Date</th>
              <th>Tournament</th>
              <th>Type</th>
              <th>Place</th>
              <th>ΔR</th>
              <th>Replay</th>
              <th>Participants &amp; Scores</th>
            </tr>
          </thead>
          <tbody>
            {state.kind === 'loading' && (
              <tr>
                <td colSpan={7} className="cell-empty">
                  Loading games…
                </td>
              </tr>
            )}
            {state.kind === 'error' && (
              <tr>
                <td colSpan={7} className="cell-empty">
                  {state.message}
                </td>
              </tr>
            )}
            {data !== null && data.matches.length === 0 && (
              <tr>
                <td colSpan={7} className="cell-empty">
                  No recent match history found.
                </td>
              </tr>
            )}
            {data?.matches.map((m) => (
              <tr key={m.match_id}>
                <td className="cell-date">{m.date}</td>
                <td className="cell-tournament">{m.tournament}</td>
                <td>
                  <span className="badge badge-seats">{m.player_count}P</span>
                </td>
                <td>
                  <span className={`placement-badge place-${m.placement}`}>{m.placement}</span>
                </td>
                <td>
                  <GameDelta value={m.rating_delta} />
                </td>
                <td>
                  <Replay code={m.replay_code} />
                </td>
                <td>
                  <div className="seats">
                    {m.seats.map((s, i) => (
                      <span key={`${s.name}-${i}`} className={s.name === data.name ? 'seat seat-self' : 'seat'}>
                        {s.name === data.name ? (
                          s.name
                        ) : (
                          <PlayerLink name={s.name} className="player-link" />
                        )}{' '}
                        <span className="seat-score">({score(s.score)})</span>
                        {i < m.seats.length - 1 ? <span className="seat-score">,</span> : ''}
                      </span>
                    ))}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollTable>
      {data !== null && data.pages > 1 && <Pages page={data.page} pages={data.pages} view={view} linkTo={linkTo} go={go} />}
    </div>
  );
}

function GameDelta({ value }: { value: number | null }) {
  const kind = value === null || value === 0 ? '' : value > 0 ? ' up' : ' down';
  return <span className={`game-delta${kind}`}>{signedDelta(value)}</span>;
}

function Replay({ code }: { code: string | null }) {
  const [copied, setCopied] = useState(false);
  if (code === null) return <span className="replay-none">—</span>;
  const copy = () => {
    if (!navigator.clipboard) return;
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <div className="replay">
      <code>{code}</code>
      <button type="button" className="btn-copy-code" onClick={copy} title="Copy replay code" aria-label="Copy replay code">
        {copied ? '✓' : '📋'}
      </button>
    </div>
  );
}

function Pages({
  page,
  pages,
  view,
  linkTo,
  go,
}: {
  page: number;
  pages: number;
  view: PlayerView;
  linkTo: LinkTo;
  go: (next: PlayerView) => void;
}) {
  const [typed, setTyped] = useState(String(page));
  useEffect(() => setTyped(String(page)), [page]);
  const to = (n: number) => linkTo({ ...view, matchPage: n });
  const jump = () => {
    const n = Number(typed);
    if (Number.isInteger(n) && n >= 1 && n <= pages) go({ ...view, matchPage: n });
  };
  return (
    <div className="pagination-container">
      {page > 1 && (
        <>
          <a {...to(1)} className="btn-antique">
            « First
          </a>
          <a {...to(page - 1)} className="btn-antique">
            ‹ Prev
          </a>
        </>
      )}
      <div className="pagination-jump">
        <span>Page</span>
        <input
          type="number"
          min={1}
          max={pages}
          value={typed}
          aria-label="Page number"
          onChange={(e) => setTyped(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && jump()}
        />
        <span>of {pages}</span>
        <button type="button" className="btn-antique" onClick={jump}>
          Go
        </button>
      </div>
      {page < pages && (
        <>
          <a {...to(page + 1)} className="btn-antique">
            Next ›
          </a>
          <a {...to(pages)} className="btn-antique">
            Last »
          </a>
        </>
      )}
    </div>
  );
}

const MIN_GAMES = [1, 2, 3, 5, 10, 20];

type Compare = { apiUrl: string; runId: number; format: Format };

function Rivals({ state, compare }: { state: Load<PlayerRivals>; compare: Compare | null }) {
  const [rv, setRv] = useState<RivalsView>({ filter: '', minGames: 1, expanded: false, sort: null });
  const [versus, setVersus] = useState<string | null>(null);
  const all = state.kind === 'ready' ? state.data.rivals : [];
  const rows = visibleRivals(all, rv);
  const filtering = rv.filter.trim() !== '' || rv.minGames > 1;
  const sortBy = (key: RivalKey) =>
    setRv({ ...rv, sort: { key, asc: rv.sort?.key === key ? !rv.sort.asc : firstSort(key) } });
  const ind = (key: RivalKey) => (
    <span className={rv.sort?.key === key ? 'sort-ind active' : 'sort-ind'}>
      {rv.sort?.key === key ? (rv.sort.asc ? '▲' : '▼') : '↕'}
    </span>
  );
  const head = (key: RivalKey, label: string, tip: string, extra = '') => (
    <button type="button" className={`sort-head${extra}`} title={tip} onClick={() => sortBy(key)}>
      {label} {ind(key)}
    </button>
  );

  return (
    <div className="rating-card">
      <div className="panel-head">
        <h3>Head-to-Head vs Rivals ({filtering ? rows.length : all.length})</h3>
        <div className="rivals-controls">
          <select
            aria-label="Minimum matches"
            value={rv.minGames}
            onChange={(e) => setRv({ ...rv, minGames: Number(e.target.value) })}
          >
            {MIN_GAMES.map((n) => (
              <option key={n} value={n}>
                {n === 1 ? 'Matches: All (1+)' : `Matches: ${n}+`}
              </option>
            ))}
          </select>
          <FilterInput
            label="Filter rivals by name"
            placeholder="🔍 Filter rival by name..."
            className="rival-filter"
            value={rv.filter}
            onChange={(filter) => setRv({ ...rv, filter })}
          />
        </div>
      </div>
      <ScrollTable hint="full opponent stats">
        <table className="data-table">
          <caption className="visually-hidden">Record against each opponent</caption>
          <thead>
            <tr>
              <th>{head('name', 'Opponent', 'Sort by Opponent Name')}</th>
              <th>{head('games', 'Games', 'Sort by Total Games')}</th>
              <th>
                {head('wins', 'W', 'Click to sort by Wins', ' win')} - {head('losses', 'L', 'Click to sort by Losses', ' loss')} -{' '}
                {head('draws', 'D', 'Click to sort by Draws', ' draw')}
              </th>
              <th>{head('win_rate', 'Win %', 'Sort by Win %')}</th>
              <th>Head-to-Head</th>
            </tr>
          </thead>
          <tbody>
            {state.kind !== 'ready' && (
              <tr>
                <td colSpan={5} className="cell-empty">
                  {state.kind === 'loading' ? 'Loading rivals…' : state.message}
                </td>
              </tr>
            )}
            {state.kind === 'ready' && all.length === 0 && (
              <tr>
                <td colSpan={5} className="cell-empty">
                  No head-to-head records found.
                </td>
              </tr>
            )}
            {rows.map((r) => (
              <tr key={r.name}>
                <td>
                  <PlayerLink name={r.name} className="player-link rival-name" />
                </td>
                <td>
                  <strong>{r.games}</strong>
                </td>
                <td>
                  <span className="wld-num win">{r.wins}</span> - <span className="wld-num loss">{r.losses}</span> -{' '}
                  <span className="wld-num draw">{r.draws}</span>
                </td>
                <td>
                  <strong className={r.win_rate >= 50 ? 'rival-rate good' : 'rival-rate bad'}>{r.win_rate.toFixed(1)}%</strong>
                </td>
                <td>
                  <button type="button" className="btn-h2h-compare" onClick={() => setVersus(r.name)}>
                    <span aria-hidden="true">⚔️</span> Compare
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollTable>
      {versus !== null && compare !== null && state.kind === 'ready' && (
        <VersusDialog
          apiUrl={compare.apiUrl}
          runId={compare.runId}
          name={state.data.name}
          other={versus}
          format={compare.format}
          onClose={() => setVersus(null)}
        />
      )}
      {all.length > RIVALS_SHOWN && !filtering && (
        <div className="rivals-expand">
          <button type="button" className="btn-antique" onClick={() => setRv({ ...rv, expanded: !rv.expanded })}>
            {rv.expanded ? `- Show top ${RIVALS_SHOWN} rivals only` : `+ Show all ${all.length} rivals`}
          </button>
        </div>
      )}
    </div>
  );
}
