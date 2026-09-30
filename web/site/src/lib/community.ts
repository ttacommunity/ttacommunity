/** The community leaderboard page's logic, as upstream's route: the edition and view, the best 8, tournament sums and the title mark. */
import type { CommunityEdition, CommunityRow } from '@ttac/shared/ratings';
import type { Edition, Tournament } from './community-editions';

export type CommunityViewMode = 'standard' | 'expanded';

export interface CommunityView {
  edition: CommunityEdition;
  view: CommunityViewMode;
  search: string;
  page: number;
  perPage: number;
}

const ALIASES: Record<string, CommunityEdition> = {
  '2026': '2026', q2_2026: '2026', current: '2026',
  '2025': '2025', y2025: '2025',
  '2024': '2024', y2024: '2024',
  race26: 'race26', race: 'race26', race2026: 'race26', '2026race': 'race26',
}; // prettier-ignore

/** parseCommunityView reads the page's address as upstream's route reads its query. */
export function parseCommunityView(params: URLSearchParams): CommunityView {
  const raw = (params.get('edition') ?? params.get('year') ?? '2026').trim().toLowerCase();
  const view = (params.get('view') ?? 'standard').toLowerCase();
  const per = (params.get('per_page') ?? '50').trim();
  const perNumber = parseInt(per, 10);
  const pageNumber = parseInt((params.get('page') ?? '1').trim(), 10);
  return {
    edition: ALIASES[raw] ?? '2026',
    view: view === 'expanded' ? 'expanded' : 'standard',
    search: (params.get('q') ?? '').trim(),
    page: isNaN(pageNumber) ? 1 : Math.max(1, pageNumber),
    perPage: per === 'all' ? 2000 : isNaN(perNumber) ? 50 : Math.max(10, Math.min(200, perNumber)),
  };
}

/** countingColumns returns a player's 8 best events, equal scores in the sheet's column order. */
export function countingColumns(scores: Record<string, number>, sheetOrder: readonly string[]): string[] {
  return sheetOrder
    .filter((c) => scores[c] !== undefined)
    .sort((a, b) => (scores[b] ?? 0) - (scores[a] ?? 0))
    .slice(0, 8);
}

/** tournamentSums returns each tournament's positive points and whether one of its events counts; null when none. */
export function tournamentSums(
  scores: Record<string, number>,
  summary: readonly Tournament[],
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

/** titleMark is the title column: WC for a World Champion or the edition's champion, then GM, M, P or G; anything else shows none, as upstream. */
export function titleMark(row: CommunityRow, edition: Edition): 'WC' | 'GM' | 'M' | 'P' | 'G' | null {
  if (row.title === 'WC' || row.name.toLowerCase() === edition.info.wc.toLowerCase()) return 'WC';
  return row.title === 'GM' || row.title === 'M' || row.title === 'P' || row.title === 'G' ? row.title : null;
}

/** pageWindow returns the page links upstream shows: three either side of the page. */
export function pageWindow(page: number, total: number): number[] {
  const got: number[] = [];
  for (let p = Math.max(1, page - 3); p < Math.min(total + 1, page + 4); p++) got.push(p);
  return got;
}

/** matchesSearch reports whether a name holds the search, ignoring case, as upstream's LIKE. */
export function matchesSearch(name: string, search: string): boolean {
  return name.toLowerCase().includes(search.toLowerCase());
}

/** whole formats points as upstream's "{:,.0f}". */
export function whole(n: number): string {
  return n.toLocaleString('en-US', { maximumFractionDigits: 0 });
}
