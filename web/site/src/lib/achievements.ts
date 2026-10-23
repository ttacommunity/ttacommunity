/** The achievements page's logic, as upstream's template and script: the honour roll, title badge, cells, filter and sorting. */
import type { AchievementSummary, Player, TournamentRecord } from '@ttac/shared/ratings';
import { titleName } from './leaderboard-display';

export type Direction = 'asc' | 'desc';

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

export const CATEGORY_ICON: Record<RecordCategory, string> = { wcs: '👑', major: '🏆', ladder: '⚡', slam: '🎾', other: '🔥' };
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
  [/^australian open/i, { '2020': 1982, '2021': 2323, '2022': 3292, '2023': 4097, '2024': 4670, '2025': 5233, '2026': 5703 }],
  [/^french open/i, { '2025': 5459, '2026': 5860 }],
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


/** formatTournamentDisplay cleans and standardizes raw tournament and stage strings. */
export function formatTournamentDisplay(raw: string): string {
  let s = (raw || '').trim();
  if (!s) return '';

  const rl3w = /^RL_3w_tb_s0*(\d+)(?:\s+(.*))?$/i.exec(s);
  if (rl3w) {
    const num = Number(rl3w[1]);
    const extra = rl3w[2]?.trim();
    return `Royal League Season ${num} 3-Way Tiebreak${extra ? ` (${extra})` : ''}`;
  }

  const rl2w = /^RL_2w_tb_s0*(\d+)$/i.exec(s);
  if (rl2w) return `Royal League Season ${Number(rl2w[1])} 2-Way Tiebreak`;

  const rltb = /^RL_tb_(?:s0*)?(\d+)$/i.exec(s);
  if (rltb) return `Royal League Season ${Number(rltb[1])} Tiebreaks`;

  const rlsGame = /^RL_s0*(\d+)\s*-\s*(.*)$/i.exec(s);
  if (rlsGame) {
    const num = Number(rlsGame[1]);
    let rest = rlsGame[2].trim();
    rest = rest.replace(/\bgame\s+(\d+)\b/i, 'Game $1');
    return `Royal League Season ${num} - ${rest}`;
  }

  const rls = /^RL_s0*(\d+)$/i.exec(s);
  if (rls) return `Royal League Season ${Number(rls[1])}`;

  const nls = /^NL_s0*(\d+)$/i.exec(s);
  if (nls) return `Nations League Season ${Number(nls[1])}`;

  const ics = /^IC_s0*(\d+)$/i.exec(s);
  if (ics) return `International Championship Season ${Number(ics[1])}`;

  const ims = /^IM_s0*(\d+)$/i.exec(s);
  if (ims) return `Intermezzo Season ${Number(ims[1])}`;

  const worldsStage = /^Worlds\s+(\d+)(?:\s*-\s*|\s+)stage\s+(\d+)$/i.exec(s);
  if (worldsStage) return `World Championship ${worldsStage[1]} - Stage ${worldsStage[2]}`;

  const worlds = /^Worlds\s+(\d+)$/i.exec(s);
  if (worlds) return `World Championship ${worlds[1]}`;

  const tcl = /^TCL\s+(Round|Stage)\s+(\d+)$/i.exec(s);
  if (tcl) return `Transcontinental Ladder ${tcl[1]} ${tcl[2]}`;

  const wtc = /^WTC2P\s+(\d+)\s+SEASON\s+(.*?)\s+Stage\s+(\d+)$/i.exec(s);
  if (wtc) return `WTC 2P ${wtc[1]} Season ${wtc[2]} - Stage ${wtc[3]}`;

  s = s.replace(/\bstage\s+(\d+)\b/gi, 'Stage $1');
  s = s.replace(/\bgame\s+(\d+)\b/gi, 'Game $1');

  return s;
}

/** isQnD reports whether a tournament is Quick and Dirty. */
export function isQnD(tournament: string): boolean {
  return /quick and dirty|q&d|q and d/i.test(tournament);
}

/** canonicalTournamentName maps raw tournament names and variants to a single canonical tournament name. */
export function canonicalTournamentName(raw: string): string {
  const s = (raw || '').trim();
  if (!s) return '';
  const lower = s.toLowerCase();
  if (lower.startsWith('world championship') || lower.startsWith('worlds')) {
    return 'World Championship';
  }
  if (lower.startsWith('royal league') || lower.startsWith('rl_') || lower.startsWith('rl ')) {
    return 'Royal League';
  }
  if (isQnD(s)) {
    return 'Quick and Dirty';
  }
  if (lower.includes('international') && !lower.includes('team')) {
    return 'International Championship';
  }
  if (lower.includes('intermezzo')) {
    return 'Intermezzo Championship';
  }
  if (lower.startsWith('nations league') || lower.startsWith('nl_')) {
    return 'Nations League';
  }
  if (lower.startsWith('tcl') || lower.includes('transcontinental')) {
    return 'Transcontinental Ladder';
  }
  if (lower.includes('mercurial') || lower.startsWith('ml_')) {
    return 'Mercurial Ladder';
  }
  if (lower.includes('sodium')) {
    return 'Sodium Ladder';
  }
  if (lower.includes('team championship')) {
    return lower.includes('international') ? 'International Team Championship' : 'Team Championship';
  }
  if (lower.includes('two player') || lower.includes('2 player')) {
    return 'Two Player Leaderboard';
  }
  return formatTournamentDisplay(s)
    .replace(/\b(season|round|stage)\s+\d+\b/gi, '')
    .replace(/\b20\d\d\b/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

/** seasonDisplay formats a record's season/stage within its canonical tournament. */
export function seasonDisplay(r: TournamentRecord): string {
  const t = r.tournament.trim();
  const lower = t.toLowerCase();
  if (lower.startsWith('world championship') || lower.startsWith('worlds')) {
    const yearMatch = /\b(20\d\d)\b/.exec(t);
    if (yearMatch) {
      return r.season && r.season !== 'Championship' && r.season !== yearMatch[1]
        ? `${yearMatch[1]} (${r.season})`
        : yearMatch[1];
    }
    const edMatch = /\b(?:championship|worlds)\s+(\d+)\b/i.exec(t);
    if (edMatch) {
      return r.season && r.season !== 'Championship'
        ? `Edition ${edMatch[1]} (${r.season})`
        : `Edition ${edMatch[1]}`;
    }
  }
  const rlTb = /^RL_(?:3w_|2w_)?tb_s0*(\d+)/i.exec(t);
  if (rlTb) {
    return `Season ${Number(rlTb[1])} Tiebreaks`;
  }
  const qndStage = /stage\s*(\d+)/i.exec(`${t} ${r.season} ${r.division}`);
  if (isQnD(t) && qndStage) {
    return r.season ? `${r.season} (Stage ${qndStage[1]})` : `Stage ${qndStage[1]}`;
  }
  return r.season || 'Tournament';
}

export type TournamentCategoryFilter = 'all' | 'major' | 'slam' | 'ladder' | 'community';

export function tournamentCategory(tournament: string): 'major' | 'slam' | 'ladder' | 'community' {
  const t = tournament.toLowerCase();
  if (
    t.includes('world championship') ||
    t.includes('worlds') ||
    (t.includes('international') && !t.includes('team')) ||
    t.includes('intermezzo') ||
    t.includes('royal league') ||
    t.startsWith('rl_') ||
    t.startsWith('rl ')
  ) {
    return 'major';
  }
  if (
    t.includes('wimbledon') ||
    t.includes('open') ||
    t.includes('slam') ||
    t.includes('cup') ||
    t.includes('trophy') ||
    t.includes('slow burn') ||
    t.includes('chakras') ||
    t.includes('bounty hunter') ||
    t.includes('autumn league')
  ) {
    return 'slam';
  }
  if (
    t.includes('ladder') ||
    t.includes('tcl') ||
    t.includes('transcontinental') ||
    t.includes('sodium') ||
    t.includes('mercurial') ||
    t.includes('eiffel') ||
    t.includes('nations league') ||
    t.includes('premier league') ||
    t.includes('duel curve') ||
    t.includes('orchestra') ||
    t.includes('two player') ||
    t.startsWith('nl_')
  ) {
    return 'ladder';
  }
  return 'community';
}

export function isTopDivision(tournament: string, division: string): boolean {
  const d = division.toLowerCase().trim();
  const t = tournament.toLowerCase().trim();
  if (t.includes('royal league') || t.startsWith('rl_')) {
    return d.includes('emperor');
  }
  if (t.includes('international')) {
    return d.includes('premier') || d.includes('division 1') || d.includes('tier 1') || d === '1';
  }
  if (t.includes('intermezzo')) {
    return d.includes('division 1') || d.includes('tier 1') || d === '1';
  }
  return d.includes('division 1') || d.includes('tier 1') || d.includes('emperor') || d === '1';
}

export function isPromotion(r: TournamentRecord): boolean {
  if (r.career) return false;
  if (isQnD(r.tournament)) return false;
  const p = (r.placement || '').trim().toLowerCase();
  const isFirst = p.startsWith('1 /') || p === '1' || p === '1st';
  return isFirst && !isTopDivision(r.tournament, r.division);
}

export function isMedal(r: TournamentRecord): boolean {
  return r.medal !== null;
}

function parsePlacementNumber(p: string): number {
  const m = /^\s*(\d+)/.exec(p);
  return m ? Number(m[1]) : 999999;
}

const MEDAL_RANK: Record<string, number> = { gold: 3, silver: 2, bronze: 1 };

function divisionScore(d: string): number {
  const dl = d.toLowerCase();
  if (dl.includes('emperor')) return 100;
  if (dl.includes('king')) return 90;
  if (dl.includes('archduke')) return 80;
  if (dl.includes('duke')) return 70;
  if (dl.includes('marquess')) return 60;
  if (dl.includes('count')) return 50;
  if (dl.includes('viscount')) return 40;
  if (dl.includes('baron')) return 30;
  if (dl.includes('premier') || dl.includes('tier 1') || dl.includes('division 1')) return 90;
  if (dl.includes('tier 2') || dl.includes('division 2')) return 70;
  if (dl.includes('tier 3') || dl.includes('division 3')) return 50;
  return 10;
}

function compareRecords(a: TournamentRecord, b: TournamentRecord): number {
  const ma = a.medal ? MEDAL_RANK[a.medal] ?? 0 : 0;
  const mb = b.medal ? MEDAL_RANK[b.medal] ?? 0 : 0;
  if (ma !== mb) return mb - ma;

  const da = divisionScore(a.division);
  const db = divisionScore(b.division);
  if (da !== db) return db - da;

  const pa = parsePlacementNumber(a.placement);
  const pb = parsePlacementNumber(b.placement);
  if (pa !== pb) return pa - pb;

  return (b.finish_date || '').localeCompare(a.finish_date || '');
}

/** careerResult formats a career row's result: medal counts with symbols or best placement. */
export function careerResult(r: TournamentRecord): string {
  const gMatch = /(\d+)\s*(?:x\s*1st|x\s*Gold|\bGold\b|Titles?|Championships?)/i.exec(r.placement);
  const sMatch = /(\d+)\s*(?:x\s*2nd|x\s*Silver|\bSilver\b|Runner-?ups?)/i.exec(r.placement);
  const bMatch = /(\d+)\s*(?:x\s*3rd|x\s*Bronze|\bBronze\b)/i.exec(r.placement);

  const g = gMatch ? Number(gMatch[1]) : 0;
  const s = sMatch ? Number(sMatch[1]) : 0;
  const b = bMatch ? Number(bMatch[1]) : 0;

  if (g > 0 || s > 0 || b > 0) {
    const parts: string[] = [];
    if (g > 0) parts.push(`🥇 ${g}`);
    if (s > 0) parts.push(`🥈 ${s}`);
    if (b > 0) parts.push(`🥉 ${b}`);
    return parts.join('   ');
  }

  if (r.medal) {
    const icon = { gold: '🥇', silver: '🥈', bronze: '🥉' }[r.medal];
    return `${icon} ${r.placement || 'Medalist'}`;
  }

  return r.placement || '-';
}

/** careerCells returns a career row's cells as the career table shows them. */
export function careerCells(r: TournamentRecord): string[] {
  return [r.tournament, r.season, r.division, careerResult(r), r.details];
}

function compareCareerResults(a: TournamentRecord, b: TournamentRecord): number {
  const parseMedals = (r: TournamentRecord) => {
    const gMatch = /(\d+)\s*(?:x\s*1st|x\s*Gold|\bGold\b|Titles?|Championships?)/i.exec(r.placement);
    const sMatch = /(\d+)\s*(?:x\s*2nd|x\s*Silver|\bSilver\b|Runner-?ups?)/i.exec(r.placement);
    const bMatch = /(\d+)\s*(?:x\s*3rd|x\s*Bronze|\bBronze\b)/i.exec(r.placement);
    return [
      gMatch ? Number(gMatch[1]) : r.medal === 'gold' ? 1 : 0,
      sMatch ? Number(sMatch[1]) : r.medal === 'silver' ? 1 : 0,
      bMatch ? Number(bMatch[1]) : r.medal === 'bronze' ? 1 : 0,
    ];
  };
  const [ga, sa, ba] = parseMedals(a);
  const [gb, sb, bb] = parseMedals(b);
  if (ga !== gb) return ga - gb;
  if (sa !== sb) return sa - sb;
  if (ba !== bb) return ba - bb;
  return parsePlacementNumber(b.placement) - parsePlacementNumber(a.placement);
}

/** sortCareer sorts career rows by one column. */
export function sortCareer(rows: TournamentRecord[], col: number, dir: Direction): TournamentRecord[] {
  return [...rows].sort((ra, rb) => {
    if (col === 3) {
      const diff = compareCareerResults(ra, rb);
      return dir === 'asc' ? diff : -diff;
    }
    const a = careerCells(ra)[col] ?? '';
    const b = careerCells(rb)[col] ?? '';
    return compare(a, b, dir, bothNumbers(a, b, a, b));
  });
}

/** bestResultsPerTournament returns the single highest achievement per tournament. */
export function bestResultsPerTournament(records: TournamentRecord[]): TournamentRecord[] {
  const seasonRecords = records.filter((r) => !r.career);
  const groups = new Map<string, TournamentRecord[]>();
  for (const r of seasonRecords) {
    const key = canonicalTournamentName(r.tournament);
    const existing = groups.get(key) ?? [];
    existing.push(r);
    groups.set(key, existing);
  }

  const results: TournamentRecord[] = [];
  for (const group of groups.values()) {
    group.sort(compareRecords);
    if (group[0]) results.push(group[0]);
  }

  results.sort((a, b) => {
    const ca = recordCategory(a.tournament);
    const cb = recordCategory(b.tournament);
    const order: Record<string, number> = { wcs: 0, major: 1, slam: 2, ladder: 3, other: 4 };
    const diff = (order[ca] ?? 99) - (order[cb] ?? 99);
    if (diff !== 0) return diff;
    return a.tournament.localeCompare(b.tournament);
  });

  return results;
}

export type MajorBest = {
  rl: TournamentRecord | null;
  ic: TournamentRecord | null;
  im: TournamentRecord | null;
  totalMatches: number;
  winRate: number;
};

export function majorBestResults(records: TournamentRecord[]): MajorBest {
  let rlBest: TournamentRecord | null = null;
  let icBest: TournamentRecord | null = null;
  let imBest: TournamentRecord | null = null;

  let totalMatches = 0;
  let totalVictories = 0;

  for (const r of records) {
    if (r.career) continue;

    const m = /(\d+)\s+matches\s+played,\s+(\d+)\s+victories/i.exec(r.details);
    if (m && m[1] && m[2]) {
      totalMatches += Number(m[1]);
      totalVictories += Number(m[2]);
    }

    const t = r.tournament.toLowerCase();
    if (t.includes('royal league') || t.startsWith('rl_') || t.startsWith('rl ')) {
      if (!rlBest || compareRecords(r, rlBest) < 0) {
        rlBest = r;
      }
    } else if (t.includes('international')) {
      if (!icBest || compareRecords(r, icBest) < 0) {
        icBest = r;
      }
    } else if (t.includes('intermezzo')) {
      if (!imBest || compareRecords(r, imBest) < 0) {
        imBest = r;
      }
    }
  }

  const winRate = totalMatches > 0 ? (totalVictories / totalMatches) * 100 : 0;
  return { rl: rlBest, ic: icBest, im: imBest, totalMatches, winRate };
}

export type TournamentOverview = {
  tournament: string;
  category: TournamentCategoryFilter;
  seasonsCount: number;
  bestRecord: TournamentRecord;
  totalMatches: number;
  wins: number;
  losses: number;
  draws: number;
  winRate: number;
  latestDate: string;
  records: TournamentRecord[];
};

/** tournamentOverviews aggregates a player's season records into exactly one entry per tournament. */
export function tournamentOverviews(records: TournamentRecord[]): TournamentOverview[] {
  const seasonRecords = records.filter((r) => !r.career);
  const groups = new Map<string, TournamentRecord[]>();
  for (const r of seasonRecords) {
    const key = canonicalTournamentName(r.tournament);
    const existing = groups.get(key) ?? [];
    existing.push(r);
    groups.set(key, existing);
  }

  const results: TournamentOverview[] = [];
  for (const [tournament, recs] of groups.entries()) {
    recs.sort((a, b) => (b.finish_date || '').localeCompare(a.finish_date || ''));
    const sortedByRank = [...recs].sort(compareRecords);
    const bestRecord = sortedByRank[0] ?? recs[0];

    let totalMatches = 0;
    let wins = 0;
    for (const r of recs) {
      const m = /(\d+)\s+matches\s+played,\s+(\d+)\s+victories/i.exec(r.details);
      if (m && m[1] && m[2]) {
        totalMatches += Number(m[1]);
        wins += Number(m[2]);
      }
    }
    const draws = 0;
    const losses = Math.max(0, totalMatches - wins - draws);
    const winRate = totalMatches > 0 ? (wins / totalMatches) * 100 : 0;
    const latestDate = recs[0]?.finish_date || '';
    const uniqueSeasons = new Set(recs.map((r) => seasonDisplay(r)));

    results.push({
      tournament,
      category: tournamentCategory(tournament),
      seasonsCount: uniqueSeasons.size,
      bestRecord,
      totalMatches,
      wins,
      losses,
      draws,
      winRate,
      latestDate,
      records: recs,
    });
  }

  results.sort((a, b) => {
    const ca = recordCategory(a.tournament);
    const cb = recordCategory(b.tournament);
    const order: Record<string, number> = { wcs: 0, major: 1, slam: 2, ladder: 3, other: 4 };
    const diff = (order[ca] ?? 99) - (order[cb] ?? 99);
    if (diff !== 0) return diff;
    if (b.seasonsCount !== a.seasonsCount) return b.seasonsCount - a.seasonsCount;
    return a.tournament.localeCompare(b.tournament);
  });

  return results;
}

/** sortTournaments sorts tournament summary rows by column. */
export function sortTournaments(
  rows: TournamentOverview[],
  col: number,
  dir: Direction,
): TournamentOverview[] {
  return [...rows].sort((a, b) => {
    let diff = 0;
    if (col === 0) {
      diff = a.tournament.localeCompare(b.tournament);
    } else if (col === 1) {
      diff = a.seasonsCount - b.seasonsCount;
    } else if (col === 2) {
      diff = compareRecords(a.bestRecord, b.bestRecord);
      return dir === 'asc' ? -diff : diff;
    } else if (col === 3) {
      diff = a.wins - b.wins || a.totalMatches - b.totalMatches;
    }
    return dir === 'asc' ? diff : -diff;
  });
}

/** matchesTournamentFilter reports whether a tournament overview matches the filter text. */
export function matchesTournamentFilter(t: TournamentOverview, filter: string): boolean {
  if (!filter) return true;
  const f = filter.toLowerCase();
  return (
    t.tournament.toLowerCase().includes(f) ||
    formatTournamentDisplay(t.tournament).toLowerCase().includes(f) ||
    t.bestRecord.division.toLowerCase().includes(f) ||
    t.bestRecord.placement.toLowerCase().includes(f)
  );
}

