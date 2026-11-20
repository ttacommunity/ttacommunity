/** A player's page for the current run, as upstream's: ratings by model, history chart, games and rivals. */
import { Fragment, useEffect, useState } from 'react';
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
import RatingChart from './RatingChart';
import VersusDialog from './VersusDialog';
import { formatTournamentDisplay } from '../lib/achievements';
import PlayerOptionsDrawer from './PlayerOptionsDrawer';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW } from '../lib/leaderboard-view';
import {
  RIVAL_COLUMNS,
  hasProfileOptions,
  isProvisional,
  matrixHref,
  pageModel,
  parsePlayerView,
  playerHref,
  playerParams,
  renamedView,
  formatCards,
  signedDelta,
  seatName,
  seatDeltaClass,
  visibleRivals,
  RIVALS_SHOWN,
  type ChartCurve,
  type ChartMode,
  type ChartScale,
  type ChartTiers,
  type PlayerView,
  type RivalKey,
  type RivalsView,
} from '../lib/player-view';
import { achievementsHref } from '../lib/achievements';
import './PlayerPage.css';
import FilterInput from './FilterInput';
import PlayerLink from './PlayerLink';
import ScrollTable from './ScrollTable';
import Panel from './Panel';
import Panels from './Panels';
import Waiting from './Waiting';
import UnratedToggle from './UnratedToggle';
import RatedFilter from './RatedFilter';
import type { GameRatingFilter } from '../lib/career';
import Pager from './Pager';
import CopyCode from './CopyCode';
import { useLoad } from './useLoad';
import { type Link } from '../lib/navigate';
import { type Load } from '@ttac/shared/load';
import { counted, ordinal, score } from '@ttac/shared/numbers';
import Message from './Message';
import PlayerHeader from './PlayerHeader';
import { SortButton } from '@ttac/shared/SortHeader';
import { nextSort } from '@ttac/shared/sort';
import { useAddressView, type Address } from './useAddressView';
import { OptionsToggle } from './SideDrawer';
import ViewBar, { FormatPick } from './ViewBar';
import Inset from './Inset';

const ADDRESS: Address<PlayerView> = {
  parse: parsePlayerView,
  params: playerParams,
  remembers: (v) => ({ ...DEFAULT_VIEW, engine: v.engine, reset: v.reset, format: v.format }),
};

/** Shows the player named in the address. Every choice is part of the address, so a view can be shared. */
export default function PlayerPage({ apiUrl }: { apiUrl: string }) {
  const { view, go, replace, linkTo } = useAddressView(ADDRESS);
  const run = useLoad(String(view.ranking), () => currentRun(apiUrl, view.ranking).then((r) => r.run_id));
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [scale, setScale] = useState<ChartScale>('perspective');
  const [tiers, setTiers] = useState<ChartTiers>('on');
  const [mode, setMode] = useState<ChartMode>('continuous');
  const [curve, setCurve] = useState<ChartCurve>('expected');
  const [engines, setEngines] = useState<Set<string>>(() => new Set(['glicko2_daneo']));

  const chooseEngines = (next: Set<string>) => {
    if (next.size > 0) setEngines(next);
  };



  const runId = run.kind === 'ready' && view.name !== '' ? run.data : null;
  const model = pageModel(view.engine, view.reset);
  const { name, format, matchPage } = view;
  const profile = useLoad<Player>(`${runId}|${name}`, runId === null ? null : () => fetchPlayer(apiUrl, runId, name));
  const history = useLoad<PlayerHistory>(
    `${runId}|${name}|${format}`,
    runId === null ? null : () => playerHistory(apiUrl, runId, name, format),
  );
  const [gamesFilter, setGamesFilter] = useState<GameRatingFilter>({ rated: true, unrated: false });
  const games = useLoad<PlayerMatches>(
    `${runId}|${name}|${format}|${model}|${matchPage}|${gamesFilter.rated}|${gamesFilter.unrated}`,
    runId === null
      ? null
      : () =>
          !gamesFilter.rated && !gamesFilter.unrated
            ? Promise.resolve({ run_id: runId, name, format, model, page: 1, pages: 1, total: 0, matches: [] })
            : playerMatches(apiUrl, runId, name, format, model, matchPage, gamesFilter.unrated, gamesFilter.rated),
  );
  const [rivalsFilter, setRivalsFilter] = useState<GameRatingFilter>({ rated: true, unrated: false });
  const rivals = useLoad<PlayerRivals>(
    `${runId}|${name}|${format}|${rivalsFilter.rated}|${rivalsFilter.unrated}`,
    runId === null
      ? null
      : () =>
          !rivalsFilter.rated && !rivalsFilter.unrated
            ? Promise.resolve({ run_id: runId, name, format, rivals: [] })
            : playerRivals(apiUrl, runId, name, format, rivalsFilter.unrated, rivalsFilter.rated),
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
    replace(renamed);
  }, [profile, view, replace]);

  if (view.name === '') return <Message>No player was named. Choose one from the leaderboard.</Message>;
  if (run.kind === 'error') return <Waiting load={run} what="the ratings" />;
  if (profile.kind !== 'ready') return <Waiting load={profile} what="this player" />;

  const p = profile.data;
  const provisional = isProvisional(p, view.format);
  const gamesInFormat = p.games.find((g) => g.format === view.format)?.games ?? 0;
  const chartOptions = { scale, tiers, mode, curve, engines };
  const activeOptions = hasProfileOptions(view, chartOptions);
  const resetOptions = () => {
    setScale('perspective');
    setTiers('on');
    setMode('continuous');
    setCurve('expected');
    setEngines(new Set(['glicko2_daneo']));
    go({ ...view, ranking: 'community', reset: 'continuous', engine: 'glicko2_daneo', matchPage: 1 });
  };

  return (
    <div className={optionsOpen ? 'player-layout options-open' : 'player-layout'}>
      <Header player={p} view={view} provisional={provisional} gamesInFormat={gamesInFormat} />

      <Panels>
        <ViewBar
          actions={
            <OptionsToggle title="Profile options, track, reset mode, and chart controls" active={activeOptions} open={optionsOpen} onClick={() => setOptionsOpen(!optionsOpen)} />
          }
        >
          <FormatPick value={view.format} link={(format) => linkTo({ ...view, format, matchPage: 1 })} what="rating and history" />
        </ViewBar>

        <CareerBox
          player={p}
          career={career.kind === 'ready' ? career.data : null}
          summary={achievements.kind === 'ready' ? achievements.data.summary : null}
          view={view}
        />

        <FormatCards player={p} view={view} linkTo={linkTo} />

        <RatingChart history={history.kind === 'ready' ? history.data : null} options={chartOptions} setEngines={setEngines} />

          <Games
            state={games}
            view={view}
            provisional={provisional}
            gamesInFormat={gamesInFormat}
            linkTo={linkTo}
            go={go}
            ratingFilter={gamesFilter}
            onRatingFilter={(next) => {
              setGamesFilter(next);
              if (view.matchPage !== 1) go({ ...view, matchPage: 1 });
            }}
          />
          <Rivals state={rivals} compare={runId === null ? null : { apiUrl, runId, format: view.format }} ratingFilter={rivalsFilter} onRatingFilter={setRivalsFilter} />

        <PositionSeatStats
          state={seats}
          format={view.format}
          includeUnrated={includeUnrated}
          onToggleUnrated={setIncludeUnrated}
        />
      </Panels>

      {optionsOpen && (
        <PlayerOptionsDrawer
          view={view}
          linkTo={linkTo}
          chart={chartOptions}
          setScale={setScale}
          setTiers={setTiers}
          setMode={setMode}
          setCurve={setCurve}
          setEngines={chooseEngines}
          onReset={resetOptions}
          onClose={() => setOptionsOpen(false)}
        />
      )}
    </div>
  );
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
  const board = new URLSearchParams({ model: view.engine, format: String(view.format) });
  return (
    <PlayerHeader
      player={player}
      back={{ href: `/ratings/?${board}`, label: 'Competitive Leaderboard' }}
      nameExtra={
        provisional && (
          <>
            {' '}
            <span className="profile-calibrating" data-tooltip={`Provisional / Calibrating Prior: ${gamesInFormat}/15 opponents faced`}>
              ⏳
            </span>
          </>
        )
      }
      page="profile"
      hrefs={{ profile: playerHref(player.name), achievements: achievementsHref(player.name), years: matrixHref(player.name, view) }}
    />
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
              <span
                className="badge badge-rank"
                data-tooltip={r && r.rank === null ? (r.opponents < 30 ? 'Unranked: fewer than 30 opponents' : 'Unranked: inactive in the last 12 months') : undefined}
              >
                {r && r.rank !== null ? `#${r.rank}` : 'Unranked'}
              </span>
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
  ratingFilter,
  onRatingFilter,
}: {
  state: Load<PlayerMatches>;
  view: PlayerView;
  provisional: boolean;
  gamesInFormat: number;
  linkTo: LinkTo;
  go: (next: PlayerView) => void;
  ratingFilter: GameRatingFilter;
  onRatingFilter: (next: GameRatingFilter) => void;
}) {
  const data = state.kind === 'ready' ? state.data : null;
  const total = data?.total ?? gamesInFormat;
  return (
    <Panel
      id="game-history"
      title={
        <>
          Game History <span className="panel-head-count">(Recent Matches: {total})</span>
        </>
      }
      aside={
        <>
          {provisional && (
            <span
              className="badge badge-calibration"
              title="Provisional Phase: Player ratings undergo prior calibration during their first 15 matches. Match deltas are displayed normally while the rating converges."
            >
              ⏳ Prior Calibration ({gamesInFormat}/15)
            </span>
          )}
          <RatedFilter value={ratingFilter} onChange={onRatingFilter} className="unrated-toggle-end" />
        </>
      }
    >
      <ScrollTable hint="full match details">
        <table className="leaderboard-table data-table">
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
                  {!ratingFilter.rated && !ratingFilter.unrated ? 'Select Rated or Unrated above to see matches.' : 'No recent match history found.'}
                </td>
              </tr>
            )}
            {data?.matches.map((m) => (
              <tr key={m.match_id}>
                <td className="cell-date">{m.date}</td>
                <td className="cell-tournament">{formatTournamentDisplay(m.tournament)}</td>
                <td>
                  <span className="badge badge-seats">{m.player_count}P</span>
                  {!m.rated && <span className="badge badge-unrated">Unrated</span>}
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
    </Panel>
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

function Rivals({
  state,
  compare,
  ratingFilter,
  onRatingFilter,
}: {
  state: Load<PlayerRivals>;
  compare: Compare | null;
  ratingFilter: GameRatingFilter;
  onRatingFilter: (next: GameRatingFilter) => void;
}) {
  const [rv, setRv] = useState<RivalsView>({ filter: '', minGames: 1, expanded: false, sort: null });
  const [versus, setVersus] = useState<{ other: string; opener: HTMLElement } | null>(null);
  const all = state.kind === 'ready' ? state.data.rivals : [];
  const rows = visibleRivals(all, rv);
  const filtering = rv.filter.trim() !== '' || rv.minGames > 1;
  const sortBy = (col: RivalKey) => setRv({ ...rv, sort: nextSort(rv.sort, col, RIVAL_COLUMNS) });
  const head = (col: RivalKey, label: string, className?: string) => (
    <SortButton col={col} sort={rv.sort} onSort={sortBy} className={className}>
      {label}
    </SortButton>
  );
  const way = (col: RivalKey) => (rv.sort?.col === col ? (rv.sort.desc ? 'descending' : 'ascending') : undefined);

  return (
    <Panel
      id="rivals"
      title={`Head-to-Head vs Rivals (${filtering ? rows.length : all.length})`}
      aside={
        <div className="rivals-controls">
          <RatedFilter value={ratingFilter} onChange={onRatingFilter} />
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
      }
    >
      <ScrollTable hint="full opponent stats">
        <table className="leaderboard-table data-table">
          <caption className="visually-hidden">Record against each opponent</caption>
          <thead>
            <tr>
              <th scope="col" aria-sort={way('name')}>
                {head('name', 'Opponent')}
              </th>
              <th scope="col" aria-sort={way('games')}>
                {head('games', 'Games')}
              </th>
              <th scope="col">
                {head('wins', 'W', 'win')} - {head('losses', 'L', 'loss')} - {head('draws', 'D', 'draw')}
              </th>
              <th scope="col" aria-sort={way('win_rate')}>
                {head('win_rate', 'Win %')}
              </th>
              <th scope="col">Head-to-Head</th>
            </tr>
          </thead>
          <tbody>
            <Waiting load={state} what="the rivals" form="row" colSpan={5} />
            {state.kind === 'ready' && all.length === 0 && (
              <tr>
                <td colSpan={5} className="cell-empty">
{!ratingFilter.rated && !ratingFilter.unrated ? 'Select Rated or Unrated above to see rivals.' : 'No head-to-head records found.'}
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
                  <button type="button" className="btn-antique btn-compact" onClick={(e) => setVersus({ other: r.name, opener: e.currentTarget })}>
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
          initialFilter={ratingFilter}
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
    </Panel>
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
  onToggleUnrated: (next: boolean) => void;
}) {
  const data = state.kind === 'ready' ? state.data : null;
  const groups = data ? (format === 0 ? data.groups : data.groups.filter((g) => g.player_count === format)) : [];
  const overall = data?.overall;

  return (
    <Panel
      id="position-seat"
      icon="🎲"
      title="Position & Turn Order Performance"
      subtitle="Win rates, average culture scores, and finishing placement distributions by starting seat."
      aside={<UnratedToggle on={includeUnrated} onChange={onToggleUnrated} />}
    >

      {overall && overall.total_games > 0 && (
        <Inset form="figures" className="position-delta-banner">
          <div className="position-banner-metric">
            <span className="banner-metric-label">Analyzed Games:</span>
            <strong>{overall.total_games}</strong>
            <span className="banner-metric-sub">
              ({counted(overall.total_wins, 'win')}
              {overall.total_draws > 0 ? ` · ${counted(overall.total_draws, 'draw')}` : ''}
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
        </Inset>
      )}

      <ScrollTable hint="seat statistics">
        <table className="leaderboard-table data-table position-seat-table">
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
                      {group.total_draws > 0 ? ` · ${counted(group.total_draws, 'draw')}` : ''}
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
                              title={`${ordinal(p.place)} place: ${p.count} times (${p.rate.toFixed(1)}%)`}
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
    </Panel>
  );
}

