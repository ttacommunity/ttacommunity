/** A player's page for the current run, as upstream's: ratings by model, history chart, games and rivals. */
import { Fragment, useCallback, useEffect, useState, type ReactNode } from 'react';
import {
  currentRun,
  player as fetchPlayer,
  playerAchievements,
  playerCareer,
  playerHistory,
  playerMatches,
  playerRivals,
  playerSeats,
  type Format,
  type Player,
  type PlayerAchievements,
  type PlayerCareer,
  type PlayerHistory,
  type PlayerMatches,
  type PlayerRivals,
  type PlayerSeatStats,
} from '@ttac/shared/ratings';
import CareerBox from './CareerBox';
import BackLink from '@ttac/shared/BackLink';
import { ExtraBadges, Flag } from './PlayerMarks';
import RatingChart from './RatingChart';
import VersusDialog from './VersusDialog';
import { formatTournamentDisplay } from '../lib/achievements';
import ViewSelectors from './ViewSelectors';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW } from '../lib/leaderboard-view';
import {
  firstSort,
  isProvisional,
  matrixHref,
  pageModel,
  parsePlayerView,
  playerParams,
  renamedView,
  formatCards,
  signedDelta,
  seatName,
  seatDeltaClass,
  visibleRivals,
  RIVALS_SHOWN,
  type PlayerView,
  type RivalKey,
  type RivalsView,
} from '../lib/player-view';
import { achievementsHref } from '../lib/achievements';
import { recall, remember } from '../lib/remembered';
import { browserStorage } from '../lib/account';
import './PlayerPage.css';
import FilterInput from './FilterInput';
import PlayerLink from './PlayerLink';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';
import Pager from './Pager';
import CopyCode from './CopyCode';
import { useLoad } from './useLoad';
import { viewLinks, type Link } from '../lib/navigate';
import { errorText, type Load } from '@ttac/shared/load';
import { score } from '@ttac/shared/numbers';

function readView(): PlayerView {
  return parsePlayerView(new URLSearchParams(window.location.search), recall(browserStorage()));
}

/** Shows the player named in the address. Every choice is part of the address, so a view can be shared. */
export default function PlayerPage({ apiUrl }: { apiUrl: string }) {
  const [view, setView] = useState<PlayerView>(readView);
  const [run, setRun] = useState<Load<number>>({ kind: 'loading' });

  const go = useCallback((next: PlayerView) => {
    window.history.pushState(null, '', `?${playerParams(next)}`);
    remember(browserStorage(), { ...DEFAULT_VIEW, engine: next.engine, reset: next.reset, format: next.format });
    setView(next);
  }, []);
  const linkTo = viewLinks(playerParams, go);

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
    `${runId}|${name}|${format}|${model}`,
    runId === null ? null : () => playerCareer(apiUrl, runId, name, format, model),
  );
  const achievements = useLoad<PlayerAchievements>(
    `${runId}|${name}`,
    runId === null ? null : () => playerAchievements(apiUrl, runId, name),
  );
  const [includeUnrated, setIncludeUnrated] = useState(false);
  const seats = useLoad<PlayerSeatStats>(
    `${runId}|${name}|${includeUnrated}`,
    runId === null ? null : () => playerSeats(apiUrl, runId, name, includeUnrated),
  );

  useEffect(() => {
    document.title = `${view.name || 'Player'} - Player Profile | Through the Ages`;
  }, [view.name]);

  // Reached by an old name, the address takes the name the player has now.
  useEffect(() => {
    if (profile.kind !== 'ready') return;
    const renamed = renamedView(view, profile.data.name);
    if (renamed === null) return;
    window.history.replaceState(null, '', `?${playerParams(renamed)}`);
    setView(renamed);
  }, [profile, view]);

  if (view.name === '') return <Message>No player was named. Choose one from the leaderboard.</Message>;
  if (run.kind === 'error') return <Waiting load={run} what="the ratings" />;
  if (profile.kind !== 'ready') return <Waiting load={profile} what="this player" />;

  const p = profile.data;
  const provisional = isProvisional(p, view.format);
  const gamesInFormat = p.games.find((g) => g.format === view.format)?.games ?? 0;

  return (
    <>
      <Header player={p} view={view} provisional={provisional} gamesInFormat={gamesInFormat} />

      <ViewSelectors
        reset={view.reset}
        format={view.format}
        linkReset={(reset) => linkTo({ ...view, reset, matchPage: 1 })}
        linkFormat={(format) => linkTo({ ...view, format, matchPage: 1 })}
      />

      <CareerBox
        player={p}
        career={career.kind === 'ready' ? career.data : null}
        summary={achievements.kind === 'ready' ? achievements.data.summary : null}
        view={view}
      />

      <FormatCards player={p} view={view} linkTo={linkTo} />

      <RatingChart history={history.kind === 'ready' ? history.data : null} />

      <div className="tables-grid">
        <Games state={games} view={view} provisional={provisional} gamesInFormat={gamesInFormat} linkTo={linkTo} go={go} />
        <Rivals state={rivals} compare={runId === null ? null : { apiUrl, runId, format: view.format }} />
      </div>

      <PositionSeatStats
        state={seats}
        format={view.format}
        includeUnrated={includeUnrated}
        onToggleUnrated={() => setIncludeUnrated(!includeUnrated)}
      />
    </>
  );
}

function Message({ children }: { children: ReactNode }) {
  return <div className="rating-card waiting-message">{children}</div>;
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
      <div className="header-back">
        <BackLink href={`/ratings/?${board}`}>Competitive Leaderboard</BackLink>
      </div>
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
                  data-tooltip={`Provisional / Calibrating Prior: ${gamesInFormat}/15 opponents faced`}
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
        <a href={matrixHref(player.name, view)} className="btn-tournament-achievements btn-matrix">
          <span aria-hidden="true">📊</span> Career Years »
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
              {r && r.opponents > 0 ? `Opponents: ${r.opponents} | Win%: ${r.win_rate.toFixed(1)}%` : c.description}
            </div>
          </a>
        );
      })}
    </div>
  );
}

type LinkTo = (next: PlayerView) => Link;

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
              <th scope="col" data-tooltip="Estimated Game Rating Delta: An approximate standalone Glicko update against pre-month baseline ratings. Does not represent the actual monthly batch rating change (see FAQ).">
                <a href="/ratings/faq/#game-deltas" className="th-link">
                  ΔR <span className="delta-info-icon" aria-hidden="true">ⓘ</span>
                </a>
              </th>
              <th>Replay</th>
              <th>Participants &amp; Scores</th>
            </tr>
          </thead>
          <tbody>
            <Waiting load={state} what="the games" form="row" colSpan={7} />
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
                <td className="cell-tournament">{formatTournamentDisplay(m.tournament)}</td>
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
                  <CopyCode code={m.replay_code} />
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
      {data !== null && data.pages > 1 && (
        <Pager
          page={data.page}
          pages={data.pages}
          link={(matchPage) => linkTo({ ...view, matchPage })}
          go={(matchPage) => go({ ...view, matchPage })}
        />
      )}
    </div>
  );
}

function GameDelta({ value }: { value: number | null }) {
  const kind = value === null || value === 0 ? '' : value > 0 ? ' up' : ' down';
  const tip =
    value === null
      ? 'Unrated match or insufficient opponent rating data'
      : `Estimated standalone impact: ${signedDelta(value)} against opponents' current ratings today (see FAQ for why this differs from monthly batch ratings)`;
  return (
    <span className={`game-delta${kind}`} data-tooltip={tip}>
      {signedDelta(value)}
    </span>
  );
}

const MIN_GAMES = [1, 2, 3, 5, 10, 20];

type Compare = { apiUrl: string; runId: number; format: Format };

function Rivals({ state, compare }: { state: Load<PlayerRivals>; compare: Compare | null }) {
  const [rv, setRv] = useState<RivalsView>({ filter: '', minGames: 1, expanded: false, sort: null });
  const [versus, setVersus] = useState<{ other: string; opener: HTMLElement } | null>(null);
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
            <Waiting load={state} what="the rivals" form="row" colSpan={5} />
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
                  <button type="button" className="btn-h2h-compare" onClick={(e) => setVersus({ other: r.name, opener: e.currentTarget })}>
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
          other={versus.other}
          format={compare.format}
          opener={versus.opener}
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

function PositionSeatStats({
  state,
  format,
  includeUnrated,
  onToggleUnrated,
}: {
  state: Load<PlayerSeatStats>;
  format: Format;
  includeUnrated: boolean;
  onToggleUnrated: () => void;
}) {
  const data = state.kind === 'ready' ? state.data : null;
  const groups = data ? (format === 0 ? data.groups : data.groups.filter((g) => g.player_count === format)) : [];
  const overall = data?.overall;

  return (
    <div className="rating-card position-seat-card">
      <div className="position-seat-head">
        <div className="position-head-title">
          <h3>
            <span aria-hidden="true">🎲</span> Position &amp; Turn Order Performance
          </h3>
          <span className="position-head-subtitle">
            Win rates, average culture scores, and finishing placement distributions by starting seat.
          </span>
        </div>
        <div className="position-filter-toggle">
          <button
            type="button"
            className={`btn-antique unrated-toggle-btn ${includeUnrated ? 'active' : ''}`}
            onClick={onToggleUnrated}
            title={includeUnrated ? 'Showing all tournament and exhibition games' : 'Showing rated games only'}
          >
            {includeUnrated ? '🌐 Filter: All Games' : '🏆 Filter: Rated Games Only'}
          </button>
        </div>
      </div>

      {overall && overall.total_games > 0 && (
        <div className="position-delta-banner">
          <div className="position-banner-metric">
            <span className="banner-metric-label">Analyzed Games:</span>
            <strong>{overall.total_games}</strong>
            <span className="banner-metric-sub">
              ({overall.total_wins} {overall.total_wins === 1 ? 'win' : 'wins'}
              {overall.total_draws > 0 ? ` · ${overall.total_draws} ${overall.total_draws === 1 ? 'draw' : 'draws'}` : ''}
              {' '}· {overall.win_rate.toFixed(1)}%)
            </span>
          </div>
          <div className="position-banner-metric">
            <span className="banner-metric-label">Avg Culture:</span>
            <strong>{overall.avg_culture.toFixed(1)}</strong>
          </div>
          {overall.strongest_seat && (
            <div className="position-banner-metric strongest-metric">
              <span className="banner-metric-label">Strongest Position:</span>
              <strong className="delta-pos">
                {overall.strongest_seat.player_count}P {seatName(overall.strongest_seat.seat)}
              </strong>
              <span className="delta-tag delta-pos">
                {signedDelta(overall.strongest_seat.delta_win_rate)}% vs Format
              </span>
            </div>
          )}
          {overall.weakest_seat && (
            <div className="position-banner-metric weakest-metric">
              <span className="banner-metric-label">Toughest Position:</span>
              <strong className="delta-neg">
                {overall.weakest_seat.player_count}P {seatName(overall.weakest_seat.seat)}
              </strong>
              <span className="delta-tag delta-neg">
                {signedDelta(overall.weakest_seat.delta_win_rate)}% vs Format
              </span>
            </div>
          )}
        </div>
      )}

      <ScrollTable hint="seat statistics">
        <table className="data-table position-seat-table">
          <caption className="visually-hidden">
            Position and turn order win rates and placement breakdown for {data?.name ?? 'player'}
          </caption>
          <thead>
            <tr>
              <th scope="col">Turn Order</th>
              <th scope="col">Games</th>
              <th scope="col">Wins</th>
              <th scope="col">Draws</th>
              <th scope="col">Win %</th>
              <th scope="col">Δ vs Format</th>
              <th scope="col">Avg Culture</th>
              <th scope="col">Finishing Distribution (1st / 2nd / 3rd / 4th)</th>
            </tr>
          </thead>
          <tbody>
            <Waiting load={state} what="position statistics" form="row" colSpan={8} />
            {data !== null && groups.length === 0 && (
              <tr>
                <td colSpan={8} className="cell-empty">
                  No recorded games in this format.
                </td>
              </tr>
            )}
            {groups.map((group) => (
              <Fragment key={group.player_count}>
                <tr className="format-group-row">
                  <td colSpan={8} className="format-group-cell">
                    <strong>{group.player_count}-Player Tables</strong>
                    <span className="group-meta">
                      {' '}— {group.total_games} games · {group.total_wins} wins
                      {group.total_draws > 0 ? ` · ${group.total_draws} ${group.total_draws === 1 ? 'draw' : 'draws'}` : ''}
                      {' '}({group.win_rate.toFixed(1)}% win rate) · Avg Culture: {group.avg_culture.toFixed(1)}
                    </span>
                  </td>
                </tr>
                {group.seats.map((row) => {
                  const deltaClass = seatDeltaClass(row.delta_win_rate);
                  return (
                    <tr key={`${group.player_count}-${row.seat}`}>
                      <td className="seat-cell">
                        <strong>{seatName(row.seat)}</strong>
                      </td>
                      <td>{row.games}</td>
                      <td>{row.wins}</td>
                      <td>{row.draws}</td>
                      <td>
                        <strong>{row.games > 0 ? `${row.win_rate.toFixed(1)}%` : '—'}</strong>
                      </td>
                      <td>
                        {row.games > 0 ? (
                          <span className={`seat-delta-badge ${deltaClass}`}>
                            {signedDelta(row.delta_win_rate)}%
                          </span>
                        ) : (
                          '—'
                        )}
                      </td>
                      <td className="cell-culture">
                        {row.games > 0 ? row.avg_culture.toFixed(1) : '—'}
                      </td>
                      <td className="places-cell">
                        <div className="places-bars">
                          {row.places.map((p) => (
                            <span
                              key={p.place}
                              className={`place-rate-pill place-pill-${p.place}`}
                              title={`${p.place}${p.place === 1 ? 'st' : p.place === 2 ? 'nd' : p.place === 3 ? 'rd' : 'th'} place: ${p.count} times (${p.rate.toFixed(1)}%)`}
                            >
                              <span className="pill-place">{p.place}:</span> {p.count}{' '}
                              <span className="pill-pct">({p.rate.toFixed(0)}%)</span>
                            </span>
                          ))}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </Fragment>
            ))}
          </tbody>
        </table>
      </ScrollTable>
    </div>
  );
}

