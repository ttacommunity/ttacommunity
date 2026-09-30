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

/** seasonCells returns a season row's cells as the seasons table shows them. */
export function seasonCells(r: TournamentRecord): string[] {
  return [r.finish_date || 'N/A', r.tournament, r.season, r.division, r.placement, r.points || '-', medalCell(r), r.details];
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

/** sortSeasons sorts season rows by one column as upstream's script: a placement by its first number, the date as text. */
export function sortSeasons(rows: TournamentRecord[], col: number, dir: Direction): TournamentRecord[] {
  return [...rows].sort((ra, rb) => {
    const a = seasonCells(ra)[col] ?? '';
    const b = seasonCells(rb)[col] ?? '';
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
