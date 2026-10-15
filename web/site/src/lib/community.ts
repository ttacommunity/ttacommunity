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

/** codesOf writes a tier's column codes: a run of one series' seasons as "ic_24–27", others listed. */
function codesOf(ids: string[]): string {
  const parts = ids.map((id) => /^([a-z]+)_(\d+)$/.exec(id));
  const series = parts[0]?.[1];
  if (ids.length > 1 && parts.every((m) => m !== null && m[1] === series)) return `${ids[0]}–${parts.at(-1)?.[2]}`;
  return ids.join(', ');
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
  const pts = (n: number) => n.toLocaleString('en-US');
  const count = data.counting_events ?? 8;
  return {
    counting: count,
    slots: data.categories.reduce((n, c) => n + c.columns.length, 0),
    tiers: data.categories.length,
    tournaments: data.summary.length,
    lines: data.categories.map((c) => {
      const max = c.columns.map((x) => x.max_pts);
      const known = max.filter((m) => m !== null);
      const [lo, hi] = [Math.min(...known), Math.max(...known)];
      return {
        name: c.category,
        codes: codesOf(c.columns.map((x) => x.id)),
        maxPts: known.length < max.length ? null : lo === hi ? pts(hi) : `${pts(lo)}–${pts(hi)}`,
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
  return n.toLocaleString('en-US', { maximumFractionDigits: 0 });
}
