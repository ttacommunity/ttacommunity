/** The achievements page's logic, as upstream's template and script: the honour roll, title badge, cells, filter and sorting. */
import type { AchievementSummary, Player, TournamentRecord } from '@ttac/shared/ratings';
import { titleName } from './leaderboard-display';

export type Direction = 'asc' | 'desc';

const MEDALS: Record<NonNullable<TournamentRecord['medal']>, string> = {
  gold: '🥇 Gold',
  silver: '🥈 Silver',
  bronze: '🥉 Bronze',
};

/** achievementsHref returns the address of a player's achievements page. */
export function achievementsHref(name: string): string {
  return `/ratings/player/achievements/?${new URLSearchParams({ name })}`;
}

/** totalTitles returns a player's titles across the four major circuits. */
export function totalTitles(s: AchievementSummary | null): number {
  return s ? s.world_titles + s.international_titles + s.intermezzo_titles + s.royal_league_titles : 0;
}

/** honorRoll returns upstream's one-line career summary. */
export function honorRoll(name: string, s: AchievementSummary): string {
  const parts = [
    [s.world_titles, 'World Champion'],
    [s.international_titles, 'International Champion'],
    [s.intermezzo_titles, 'Intermezzo Champion'],
    [s.royal_league_titles, 'Royal League Champion'],
  ]
    .filter(([n]) => n)
    .map(([n, title]) => `${n}x ${title}`);
  const medals = `${s.gold}G / ${s.silver}S / ${s.bronze}B`;
  return parts.length
    ? `${name} is a decorated competitor with ${parts.join(', ')} (Total Career Medals: ${medals}).`
    : `${name} has achieved ${medals} podium medals.`;
}

/** titleBadge returns the badge above the name: World Champion for a current or past world title, else the current title. */
export function titleBadge(
  title: Player['title'],
  peak: Player['peak'],
): { code: string; text: string; tooltip: string; reigning: boolean } | null {
  if (title?.code === 'WC' || peak?.code === 'WC') {
    const reigning = title?.code === 'WC';
    const year = peak?.code === 'WC' && peak.year !== null ? ` (${peak.year})` : '';
    return {
      code: 'wc',
      text: 'World Champion',
      tooltip: `Official Competitive Title: ${reigning ? 'Reigning ' : ''}World Champion${year}`,
      reigning,
    };
  }
  if (title === null) return null;
  const name = titleName(title.code);
  return { code: title.code.toLowerCase(), text: name, tooltip: `Official Competitive Title: ${name}`, reigning: false };
}

function medalCell(r: TournamentRecord): string {
  return r.medal ? MEDALS[r.medal] : '-';
}

/** RecordCategory is upstream's medal style for a tournament: World Championship, major league, ladder, slam or other. */
export type RecordCategory = 'wcs' | 'major' | 'ladder' | 'slam' | 'other';

/** recordCategory files a tournament under upstream's medal styles, by its name. */
export function recordCategory(tournament: string): RecordCategory {
  const t = tournament.toLowerCase();
  if (t.includes('world championship') || t.includes('worlds')) return 'wcs';
  if (t.includes('international') || t.includes('intermezzo') || t.includes('royal league') || t.startsWith('rl_')) return 'major';
  if (t.includes('ladder') || t.startsWith('tcl ')) return 'ladder';
  if (t.includes('open') || t.includes('wimbledon') || t.includes('slam')) return 'slam';
  return 'other';
}

const CATEGORY_ICON: Record<RecordCategory, string> = { wcs: '👑', major: '🏆', ladder: '⚡', slam: '🎾', other: '🔥' };
const MEDAL_ICON = { gold: '🥇', silver: '🥈', bronze: '🥉' } as const;

/** resultCell returns a season row's result: its category and medal icons before the placement, or the placement alone. */
export function resultCell(r: TournamentRecord): string {
  if (!r.medal) return r.placement || '-';
  return `${CATEGORY_ICON[recordCategory(r.tournament)]}${MEDAL_ICON[r.medal]} ${r.placement}`;
}

/** seasonCells returns a season row's cells as the seasons table shows them. */
export function seasonCells(r: TournamentRecord): string[] {
  return [r.finish_date || 'N/A', r.tournament, r.season, r.division, resultCell(r), r.details];
}

const CGE = 'https://account.czechgames.com/tournaments/detail/';

// CGE's tournament for each of ours, from our CGE catalogue map: one page, or one page per year.
const CGE_LEAGUES: [RegExp, number][] = [
  [/^international/i, 21],
  [/^intermezzo/i, 19],
  [/^royal league|^rl_/i, 5000],
  [/^mercurial/i, 4193],
  [/^sodium/i, 5169],
  [/^tcl|^transcontinental/i, 53],
  [/^eiffel/i, 3947],
  [/^nations league/i, 4127],
  [/^premier league/i, 4566],
];
const CGE_YEARLY: [RegExp, Record<string, number>][] = [
  [/^world championship/i, { '2022': 3391, '2023': 4149, '2024': 4760, '2025': 5204, '2026': 5633, '2027': 5982 }],
  [/^wimbledon/i, { '2023': 4369, '2024': 4991, '2025': 5495, '2026': 5913 }],
  [/^australian open/i, { '2021': 2323, '2022': 3292, '2023': 4097, '2024': 4670, '2025': 5233, '2026': 5703 }],
  [/^french open/i, { '2026': 5860 }],
  [/^survivors cup/i, { '2026': 5731 }],
  [/^leaderboard trophy/i, { '2025': 5695 }],
  [/^nations cup/i, { '2025': 5373 }],
  [/^slow burn/i, { '2024': 4807, '2026': 5979 }],
];

/** cgeTournamentUrl returns the CGE page of a record's tournament (that year's, for a yearly event), or null if it has none. */
export function cgeTournamentUrl(tournament: string, season: string): string | null {
  for (const [name, id] of CGE_LEAGUES) if (name.test(tournament)) return CGE + id;
  const year = /\b(20\d\d)\b/.exec(`${tournament} ${season}`)?.[1];
  for (const [name, years] of CGE_YEARLY) {
    if (name.test(tournament)) return year !== undefined && years[year] !== undefined ? CGE + years[year] : null;
  }
  return null;
}

/** careerCells returns a career row's cells as the career table shows them. */
export function careerCells(r: TournamentRecord): string[] {
  return [r.tournament, r.season, r.division, r.placement, r.points || '-', medalCell(r), r.details];
}

/** matchesFilter reports whether a row's text holds the filter, ignoring case; cells are separate lines, as in the page's text. */
export function matchesFilter(cells: string[], filter: string): boolean {
  return cells.join('\n').toLowerCase().includes(filter.toLowerCase());
}

function number(cell: string): number {
  return parseFloat(cell.replace(/[^0-9.-]/g, ''));
}

function compare(a: string, b: string, dir: Direction, numeric: boolean): number {
  const [x, y] = dir === 'asc' ? [a, b] : [b, a];
  if (numeric) return number(x) - number(y);
  return x.localeCompare(y, undefined, { numeric: true, sensitivity: 'base' });
}

function bothNumbers(a: string, b: string, va: string, vb: string): boolean {
  return !isNaN(number(va)) && !isNaN(number(vb)) && /^[0-9]/.test(a) && /^[0-9]/.test(b);
}

/** sortSeasons sorts season rows by one column as upstream's script: the result by its place, the date as text. */
export function sortSeasons(rows: TournamentRecord[], col: number, dir: Direction): TournamentRecord[] {
  return [...rows].sort((ra, rb) => {
    const a = seasonCells(ra)[col] ?? '';
    const b = seasonCells(rb)[col] ?? '';
    if (col === 4) {
      const ka = Number(/\d+/.exec(a)?.[0] ?? 999999);
      const kb = Number(/\d+/.exec(b)?.[0] ?? 999999);
      if (ka !== kb) return dir === 'asc' ? ka - kb : kb - ka;
    }
    const va = a.includes('/') ? (a.split('/')[0] ?? '').trim() : a;
    const vb = b.includes('/') ? (b.split('/')[0] ?? '').trim() : b;
    if (col !== 0 && bothNumbers(a, b, va, vb)) return compare(va, vb, dir, true);
    return compare(a, b, dir, false);
  });
}

/** sortCareer sorts career rows by one column as upstream's script: numbers by value, anything else as text. */
export function sortCareer(rows: TournamentRecord[], col: number, dir: Direction): TournamentRecord[] {
  return [...rows].sort((ra, rb) => {
    const a = careerCells(ra)[col] ?? '';
    const b = careerCells(rb)[col] ?? '';
    return compare(a, b, dir, bothNumbers(a, b, a, b));
  });
}
