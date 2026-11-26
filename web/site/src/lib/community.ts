import { count } from '@ttac/shared/numbers';
import type {
  CommunityEditionItem,
  CommunityLeaderboard,
  CommunityRow,
} from '@ttac/shared/ratings';

type EventColumn = CommunityLeaderboard['categories'][number]['columns'][number];

export type CommunityViewMode = 'standard' | 'expanded';

export interface CommunityView {
  edition: string;
  view: CommunityViewMode;
  search: string;
  page: number;
  perPage: number;
}

const ALIASES: Record<string, string> = {
  current: '',
  '2026': '2026', q2_2026: '2026',
  '2025': '2025', y2025: '2025',
  '2024': '2024', y2024: '2024',
  race26: 'race26', race: 'race26', race2026: 'race26', '2026race': 'race26',
}; // prettier-ignore

/** parseCommunityView reads the page's address as upstream's route reads its query; edition "" leaves it to admin. */
export function parseCommunityView(params: URLSearchParams): CommunityView {
  const raw = (params.get('edition') ?? params.get('year') ?? '').trim().toLowerCase();
  const view = (params.get('view') ?? 'standard').toLowerCase();
  const per = (params.get('per_page') ?? '50').trim();
  const perNumber = parseInt(per, 10);
  const pageNumber = parseInt((params.get('page') ?? '1').trim(), 10);
  return {
    edition: ALIASES[raw] ?? raw,
    view: view === 'expanded' ? 'expanded' : 'standard',
    search: (params.get('q') ?? '').trim(),
    page: isNaN(pageNumber) ? 1 : Math.max(1, pageNumber),
    perPage: per === 'all' ? 2000 : isNaN(perNumber) ? 50 : Math.max(10, Math.min(200, perNumber)),
  };
}

/** editionToShow returns the edition asked for, else the first that admin shows, in admin's order; null for none. */
export function editionToShow(asked: string, editions: readonly Pick<CommunityEditionItem, 'slug'>[]): string | null {
  return asked || (editions[0]?.slug ?? null);
}

/** countingColumns returns a player's best events, equal scores in the sheet's column order. */
export function countingColumns(scores: Record<string, number>, sheetOrder: readonly string[], count = 8): string[] {
  return sheetOrder
    .filter((c) => scores[c] !== undefined)
    .sort((a, b) => (scores[b] ?? 0) - (scores[a] ?? 0))
    .slice(0, count);
}

/** tournamentSums returns each tournament's positive points and whether one of its events counts; null when none. */
export function tournamentSums(
  scores: Record<string, number>,
  summary: CommunityLeaderboard['summary'],
  counting: readonly string[],
): Record<string, { sum: number; counting: boolean } | null> {
  const got: Record<string, { sum: number; counting: boolean } | null> = {};
  for (const t of summary) {
    const held = t.cols.filter((c) => (scores[c] ?? 0) > 0);
    got[t.id] = held.length
      ? { sum: held.reduce((s, c) => s + (scores[c] ?? 0), 0), counting: held.some((c) => counting.includes(c)) }
      : null;
  }
  return got;
}

/** titleMark is the title column: WC for a World Champion or the edition's champion, else the player's title, if any. */
export function titleMark(row: CommunityRow, wcName = ''): CommunityRow['title'] {
  if (row.title === 'WC' || (wcName !== '' && row.name.toLowerCase() === wcName.toLowerCase())) return 'WC';
  return row.title;
}

/** eventsOf names a tier's events: one series' seasons as "seasons 24–27", others by name, and nothing for a
 * single event named as its tier. */
function eventsOf(category: string, columns: Pick<EventColumn, 'id' | 'name'>[]): string {
  const seasons = columns.map((c) => /^([a-z]+)_(\d+)$/.exec(c.id));
  const series = seasons[0]?.[1];
  if (seasons.every((m) => m !== null && m[1] === series)) {
    const [first, last] = [Number(seasons[0]?.[2]), Number(seasons.at(-1)?.[2])];
    return first === last ? `season ${first}` : `seasons ${first}–${last}`;
  }
  if (columns.length === 1 && columns[0]?.name === category) return '';
  return columns.map((c) => c.name).join(', ');
}

/** columnTitle names an event's column, with the most points it gives when that is known. */
export function columnTitle(c: EventColumn): string {
  return c.max_pts === null ? c.name : `${c.name} (Max: ${c.max_pts} pts)`;
}

/** editionLabel is an edition's label in the edition picker: its short name, else its name. */
export function editionLabel(e: Pick<CommunityEditionItem, 'name' | 'short_name'>): string {
  return e.short_name || e.name;
}

/** editionFacts returns an edition's event slots, tiers and tournaments, and each tier's columns and maximum
 * points (null when a column's is unknown). */
export function editionFacts(data: Pick<CommunityLeaderboard, 'categories' | 'summary' | 'counting_events'>) {
  return {
    counting: data.counting_events ?? 8,
    slots: data.categories.reduce((n, c) => n + c.columns.length, 0),
    tiers: data.categories.length,
    tournaments: data.summary.length,
    lines: data.categories.map((c) => {
      const max = c.columns.map((x) => x.max_pts);
      const known = max.filter((m) => m !== null);
      const [lo, hi] = [Math.min(...known), Math.max(...known)];
      return {
        name: c.category,
        events: eventsOf(c.category, c.columns),
        maxPts: known.length < max.length ? null : lo === hi ? count(hi) : `${count(lo)}–${count(hi)}`,
        each: c.columns.length > 1,
      };
    }),
  };
}

/** matchesSearch reports whether a name holds the search, ignoring case, as upstream's LIKE. */
export function matchesSearch(name: string, search: string): boolean {
  return name.toLowerCase().includes(search.toLowerCase());
}

/** whole formats points as upstream's "{:,.0f}". */
export function whole(n: number): string {
  return n.toLocaleString(undefined, { maximumFractionDigits: 0 });
}
