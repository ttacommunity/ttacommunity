/** Upstream's leaderboard view: its options and filters, ranking, search, sort and paging, all in the browser. */
import type { Format, History, LeaderboardRow, Model, TitleCode, YearLeaderboard } from '@ttac/shared/ratings';

/** Upstream's fixed cutoff, kept for a run that recorded no last game. */
const UPSTREAM_CUTOFF = '2025-05-31';

/** inactiveCutoff returns the date on or before which a last game makes a player inactive: 12 months before the run's last game. */
export function inactiveCutoff(lastGame: string | null): string {
  if (lastGame === null) return UPSTREAM_CUTOFF;
  const d = new Date(`${lastGame}T00:00:00Z`);
  d.setUTCFullYear(d.getUTCFullYear() - 1);
  return d.toISOString().slice(0, 10);
}

/** Rows a page, and the fewest opponents a player needs to appear by default. */
export const PER_PAGE = 50;
export const MIN_OPPONENTS = 30;

/** Below this many opponents an active player is provisional. */
const PROVISIONAL_BELOW = 15;

export const ENGINES = ['glicko2_daneo', 'glicko2_std', 'glicko2_mp', 'glicko2_adapt', 'whr'] as const;
export type Engine = (typeof ENGINES)[number];
export type ResetMode = 'continuous' | 'softer';
export type Status = 'active' | 'all' | 'inactive';
export type SortKey = 'rank' | 'name' | 'c_rating' | 'rating' | 'opps' | 'win_rate' | 'last_played';
export type Order = 'asc' | 'desc';
export type DeltaWindow = 'none' | 'last_update' | 'month' | 'quarter' | 'year';
export type StatsView = 'season' | 'career';

/** Upstream's fixed delta dates, kept for a run that recorded no last game. */
const UPSTREAM_DELTA_DATES = { last_update: null, month: '2026-05-01', quarter: '2026-03-01', year: '2025-05-31' } as const;

/**
 * deltaDates returns the date each delta window compares against: 30, 90 and 365 days before the run's last game, or
 * null for the previous rating period.
 */
export function deltaDates(lastGame: string | null): Record<Exclude<DeltaWindow, 'none'>, string | null> {
  if (lastGame === null) return { ...UPSTREAM_DELTA_DATES };
  const before = (days: number) => new Date(Date.parse(`${lastGame}T00:00:00Z`) - days * 86_400_000).toISOString().slice(0, 10);
  return { last_update: null, month: before(30), quarter: before(90), year: before(365) };
}
const DELTAS: readonly DeltaWindow[] = ['none', 'last_update', 'month', 'quarter', 'year'];

const FORMATS: readonly Format[] = [0, 2, 3, 4];
const STATUSES: readonly Status[] = ['active', 'all', 'inactive'];
const SORTS: readonly SortKey[] = ['rank', 'name', 'c_rating', 'rating', 'opps', 'win_rate', 'last_played'];
// Upstream orders GM, M, P and G; the rest come in its database's order, which both its sites show as this.
const TITLES: readonly TitleCode[] = ['GM', 'M', 'P', 'G', 'SGM', 'WC', 'S', 'W', 'B'];

/** Everything the reader has chosen, as upstream's query parameters carry it. */
export interface View {
  engine: Engine;
  reset: ResetMode;
  retro: boolean;
  format: Format;
  status: Status;
  title: TitleCode | null;
  minOpps: number;
  delta: DeltaWindow;
  search: string;
  sort: SortKey;
  order: Order;
  page: number;
  /** A year's archive, or null for the live board. */
  year: number | null;
  /** In a year's archive, the record shown: that year's, or the career up to it. */
  stats: StatsView;
}

/** The choices remembered between visits, as upstream's session remembers them. */
export type Remembered = Partial<Pick<View, 'engine' | 'reset' | 'retro' | 'format'>>;

export const DEFAULT_VIEW: View = {
  engine: 'glicko2_daneo',
  reset: 'continuous',
  retro: false,
  format: 0,
  status: 'active',
  title: null,
  minOpps: MIN_OPPONENTS,
  delta: 'none',
  search: '',
  sort: 'rank',
  order: 'asc',
  page: 1,
  year: null,
  stats: 'season',
};

/** defaultMinOpps is upstream's default minimum opponents: 30 live, none in a year's archive. */
export function defaultMinOpps(year: number | null): number {
  return year === null ? MIN_OPPONENTS : 0;
}

export type RankedRow = LeaderboardRow & { displayRank: number; active: boolean; provisional: boolean };

function oneOf<T>(values: readonly T[], value: unknown): T | undefined {
  return values.find((v) => String(v) === value);
}

function whole(value: string | null, min: number): number | undefined {
  if (value === null || !/^\d+$/.test(value)) return undefined;
  const n = Number(value);
  return n >= min ? n : undefined;
}

/** parseView reads a view from upstream's parameters, then remembered choices, then the defaults. */
export function parseView(params: URLSearchParams, remembered: Remembered): View {
  const get = (k: string) => params.get(k);
  const retro = get('retro');
  const order = oneOf<Order>(['asc', 'desc'], get('order'));
  const y = get('year') ?? '';
  const year = /^\d{4}$/.test(y) && Number(y) >= 2000 && Number(y) <= 2100 ? Number(y) : null;
  return {
    engine: oneOf(ENGINES, get('model')) ?? remembered.engine ?? DEFAULT_VIEW.engine,
    reset: oneOf<ResetMode>(['continuous', 'softer'], get('reset_mode')) ?? remembered.reset ?? DEFAULT_VIEW.reset,
    retro: retro === '1' ? true : retro === '0' ? false : (remembered.retro ?? DEFAULT_VIEW.retro),
    format: oneOf(FORMATS, get('format')) ?? remembered.format ?? DEFAULT_VIEW.format,
    status: oneOf(STATUSES, get('status')) ?? DEFAULT_VIEW.status,
    title: oneOf(TITLES, get('title')) ?? null,
    minOpps: whole(get('min_opps'), 0) ?? defaultMinOpps(year),
    delta: oneOf(DELTAS, get('delta')) ?? DEFAULT_VIEW.delta,
    search: (get('search') ?? '').trim(),
    sort: oneOf(SORTS, get('sort')) ?? DEFAULT_VIEW.sort,
    order: order ?? DEFAULT_VIEW.order,
    page: whole(get('page'), 1) ?? DEFAULT_VIEW.page,
    year,
    stats: oneOf<StatsView>(['season', 'career'], get('stats_view')) ?? DEFAULT_VIEW.stats,
  };
}

/**
 * viewParams writes a view as upstream's parameters, leaving out what equals the defaults. An explicit link always
 * names the remembered choices, so it shows the same view in any browser.
 */
export function viewParams(view: View, explicit = false): URLSearchParams {
  const p = new URLSearchParams();
  const remembered = new Set(['model', 'reset_mode', 'retro', 'format']);
  const put = (k: string, v: string, d: string) => (v !== d || (explicit && remembered.has(k))) && p.set(k, v);
  put('model', view.engine, DEFAULT_VIEW.engine);
  put('reset_mode', view.reset, DEFAULT_VIEW.reset);
  put('retro', view.retro ? '1' : '0', '0');
  put('format', String(view.format), String(DEFAULT_VIEW.format));
  put('status', view.status, DEFAULT_VIEW.status);
  put('title', view.title ?? '', '');
  put('min_opps', String(view.minOpps), String(defaultMinOpps(view.year)));
  put('delta', view.delta, DEFAULT_VIEW.delta);
  put('search', view.search, '');
  put('sort', view.sort, DEFAULT_VIEW.sort);
  put('order', view.order, DEFAULT_VIEW.order);
  put('page', String(view.page), '1');
  put('year', view.year === null ? '' : String(view.year), '');
  put('stats_view', view.stats, DEFAULT_VIEW.stats);
  return p;
}

/** retroApplies reports whether the engine has the retrospective-prior variant. */
export function retroApplies(engine: Engine): boolean {
  return engine !== 'glicko2_daneo' && engine !== 'whr';
}

/** modelOf returns the stored model a view reads: engine, then season reset, then the retro prior. */
export function modelOf(view: View): Model {
  const softer = view.reset === 'softer' ? '_softer' : '';
  const retro = view.retro && retroApplies(view.engine) ? '_retro' : '';
  return `${view.engine}${softer}${retro}` as Model;
}

const byName = (a: { name: string }, b: { name: string }) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);

/**
 * rankPool keeps the players the status, opponent and title filters allow, ranked 1… by conservative rating. Active
 * means a last game after cutoff (inactiveCutoff), or in a year's archive, being among those who played that year.
 */
export function rankPool(
  rows: readonly LeaderboardRow[],
  view: View,
  playedInYear?: ReadonlySet<string>,
  cutoff: string = UPSTREAM_CUTOFF,
): RankedRow[] {
  return rows
    .map((p) => {
      const active =
        view.year !== null && playedInYear
          ? playedInYear.has(p.name)
          : p.last_played !== null && p.last_played > cutoff;
      return { ...p, displayRank: 0, active, provisional: active && p.opponents < PROVISIONAL_BELOW };
    })
    .filter(
      (p) =>
        p.opponents >= view.minOpps &&
        (view.status === 'all' || p.active === (view.status === 'active')) &&
        (view.title === null || p.title?.code === view.title),
    )
    .sort((a, b) => b.c_rating - a.c_rating || byName(a, b))
    .map((p, i) => ({ ...p, displayRank: i + 1 }));
}

function matches(p: RankedRow, term: string): boolean {
  const t = term.toLowerCase();
  return p.name.toLowerCase().includes(t) || p.country?.toLowerCase() === t;
}

/**
 * search applies upstream's search: a comma list shows its players; one term jumps to an exact name or a single
 * match, keeping the whole table; otherwise only the matches show, with their ranks.
 */
export function search(pool: RankedRow[], term: string): { rows: RankedRow[]; jumpTo: string | null } {
  if (term === '') return { rows: pool, jumpTo: null };
  if (term.includes(',')) {
    const terms = term.split(',').map((t) => t.trim()).filter((t) => t !== '');
    return { rows: pool.filter((p) => terms.some((t) => matches(p, t))), jumpTo: null };
  }
  const found = pool.filter((p) => matches(p, term));
  const exact = found.find((p) => p.name.toLowerCase() === term.toLowerCase());
  if (exact || found.length === 1) return { rows: pool, jumpTo: (exact ?? found[0]).name };
  return { rows: found, jumpTo: null };
}

const sortValue: Record<Exclude<SortKey, 'name' | 'last_played'>, (p: RankedRow) => number> = {
  rank: (p) => p.displayRank,
  c_rating: (p) => p.c_rating,
  rating: (p) => p.rating,
  opps: (p) => p.opponents,
  win_rate: (p) => p.win_rate,
};

/** sortRows orders rows by a column, ties by rank. */
export function sortRows(rows: RankedRow[], key: SortKey, order: Order): RankedRow[] {
  const cmp = (a: RankedRow, b: RankedRow): number => {
    if (key === 'name') return byName(a, b);
    if (key === 'last_played') return (a.last_played ?? '').localeCompare(b.last_played ?? '');
    return sortValue[key](a) - sortValue[key](b);
  };
  const dir = order === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => dir * cmp(a, b) || a.displayRank - b.displayRank);
}

/** nextSort is the sort a column header chooses: rank and name start ascending, the numbers descending. */
export function nextSort(view: View, key: SortKey): { sort: SortKey; order: Order } {
  const first: Order = key === 'rank' || key === 'name' ? 'asc' : 'desc';
  const flipped: Order = first === 'asc' ? 'desc' : 'asc';
  return { sort: key, order: view.sort === key && view.order === first ? flipped : first };
}

/** page returns one page of rows: the page of the player jumped to, or the page asked for, within range. */
export function page(rows: RankedRow[], asked: number, jumpTo: string | null) {
  const pages = Math.max(1, Math.ceil(rows.length / PER_PAGE));
  const at = jumpTo === null ? -1 : rows.findIndex((p) => p.name === jumpTo);
  const n = at >= 0 ? Math.floor(at / PER_PAGE) + 1 : Math.min(Math.max(asked, 1), pages);
  return { rows: rows.slice((n - 1) * PER_PAGE, n * PER_PAGE), page: n, pages, total: rows.length };
}

/** hasOptions reports whether any option differs from upstream's defaults (the Options button's "Active"). */
export function hasOptions(view: View): boolean {
  return view.engine !== DEFAULT_VIEW.engine || view.reset !== DEFAULT_VIEW.reset || hasFilters({ ...view, search: '' });
}

/** hasFilters reports whether a filter pill shows, and so "Clear Filters". */
export function hasFilters(view: View): boolean {
  return (
    view.search !== '' ||
    view.title !== null ||
    view.status !== DEFAULT_VIEW.status ||
    view.minOpps !== defaultMinOpps(view.year) ||
    view.delta !== DEFAULT_VIEW.delta
  );
}

/** suggest returns up to 12 players for the last term typed: exact, then prefix, then substring matches. */
export function suggest<T extends { name: string }>(rows: readonly T[], input: string): T[] {
  const q = (input.split(',').pop() ?? '').trim().toLowerCase();
  if (q.length < 2) return [];
  const group = (name: string) => (name.toLowerCase() === q ? 0 : name.toLowerCase().startsWith(q) ? 1 : 2);
  return rows
    .filter((p) => p.name.toLowerCase().includes(q))
    .sort((a, b) => group(a.name) - group(b.name) || byName(a, b))
    .slice(0, 12);
}

/** titleOptions lists the titles some row holds: GM, M, P and G first, as upstream orders them. */
export function titleOptions(rows: readonly LeaderboardRow[]): TitleCode[] {
  const held = new Set(rows.map((p) => p.title?.code));
  return TITLES.filter((t) => held.has(t));
}

/** Delta is a player's change over the chosen window, and their rank then (null without a past rating). */
export type Delta = { c_rating: number; rating: number; pastRank: number | null };

/**
 * deltas returns each row's rating and C-rating now minus in the past (0 for a player with no past rating), and their
 * past rank among everyone then by C-rating, as upstream ranks its history (ties by name).
 */
export function deltas(rows: readonly LeaderboardRow[], past: History['players']): Map<string, Delta> {
  const then = new Map(past.map((h) => [h.name, h]));
  const ranked = [...past].sort((a, b) => b.c_rating - a.c_rating || a.name.localeCompare(b.name));
  const pastRank = new Map(ranked.map((h, i) => [h.name, i + 1]));
  return new Map(
    rows.map((p) => {
      const h = then.get(p.name);
      return [
        p.name,
        h
          ? { c_rating: p.c_rating - h.c_rating, rating: p.rating - h.rating, pastRank: pastRank.get(p.name) ?? null }
          : { c_rating: 0, rating: 0, pastRank: null },
      ];
    }),
  );
}

/** rankChange is how many places a player rose since the window's start (past rank minus the rank shown), or 0. */
export function rankChange(d: Delta | undefined, shownRank: number | null): number {
  return d?.pastRank == null || shownRank === null ? 0 : d.pastRank - shownRank;
}

/**
 * csvUrl is the API's CSV of the table as shown, or of every player, inactive too, with the same title and search; in
 * a year's archive, that year's board in the record view shown.
 */
export function csvUrl(api: string, run: number, view: View, everyone: boolean): string {
  const q = new URLSearchParams({
    model: modelOf(view),
    format: String(view.format),
    status: everyone ? 'all' : view.status,
    min_opps: String(everyone ? 0 : view.minOpps),
  });
  if (view.title !== null) q.set('title', view.title);
  if (view.search !== '') q.set('search', view.search);
  q.set('sort', view.sort);
  q.set('order', view.order);
  if (view.year !== null) {
    q.set('year', String(view.year));
    q.set('stats_view', view.stats);
  }
  return `${api.replace(/\/+$/, '')}/v1/runs/${run}/leaderboard.csv?${q}`;
}

/**
 * yearRows turns a year's board into leaderboard rows carrying the season or career record, and names the players
 * who played that year (the archive's "active").
 */
export function yearRows(board: YearLeaderboard, stats: StatsView): { rows: LeaderboardRow[]; played: Set<string> } {
  const rows = board.players.map((p): LeaderboardRow => {
    const r = stats === 'career' ? p.career : p.season;
    return {
      rank: null,
      rank_delta: 0,
      name: p.name,
      rating: p.rating,
      rd: p.rd,
      c_rating: p.c_rating,
      games: r.opponents,
      opponents: r.opponents,
      wins: r.wins,
      losses: r.losses,
      draws: r.draws,
      win_rate: r.win_rate,
      last_played: p.last_played,
      country: p.country,
      title: p.title,
    };
  });
  return { rows, played: new Set(board.players.filter((p) => p.season.opponents > 0).map((p) => p.name)) };
}

/** yearDeltas returns each player's change since the previous year's end, and their rank then among the board's players; 0 without a rating then. */
export function yearDeltas(board: YearLeaderboard): Map<string, Delta> {
  const before = board.players.flatMap((p) => (p.previous ? [{ name: p.name, c_rating: p.previous.c_rating }] : []));
  before.sort((a, b) => b.c_rating - a.c_rating || a.name.localeCompare(b.name));
  const pastRank = new Map(before.map((h, i) => [h.name, i + 1]));
  return new Map(
    board.players.map((p) => [
      p.name,
      p.previous
        ? { c_rating: p.c_rating - p.previous.c_rating, rating: p.rating - p.previous.rating, pastRank: pastRank.get(p.name) ?? null }
        : { c_rating: 0, rating: 0, pastRank: null },
    ]),
  );
}
