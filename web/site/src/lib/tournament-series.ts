/** Upstream's Hall of Fame series pages: the series tabs, each series' text, and how its figures are written. */
import type { Legend, Series, SeriesCount } from '@ttac/shared/ratings';

/** A tab in the series bar: the Hall of Fame overview or one series. */
export interface SeriesTab {
  slug: 'overview' | Series | 'worlds' | 'grand_slams' | 'ladders';
  icon: string;
  label: string;
}

/** The series bar's tabs, in upstream's order. */
export const SERIES_NAV: SeriesTab[] = [
  { slug: 'overview', icon: '🏆', label: 'Overview' },
  { slug: 'international', icon: '🌐', label: 'International (3/4P)' },
  { slug: 'intermezzo', icon: '⚔️', label: 'Intermezzo (3P)' },
  { slug: 'royal_league', icon: '👑', label: 'Royal League (2P)' },
  { slug: 'worlds', icon: '🏆', label: 'World Championship' },
  { slug: 'grand_slams', icon: '🎾', label: 'Grand Slams' },
  { slug: 'ladders', icon: '🪜', label: 'Competitive Ladders' },
];

/** The tabs whose page exists yet; the others show without a link. */
export const BUILT: ReadonlySet<SeriesTab['slug']> = new Set([
  'overview',
  'international',
  'intermezzo',
  'royal_league',
  'worlds',
  'grand_slams',
  'ladders',
]);

/** The series that are one tournament each: their page is that tournament's. */
export const SINGLE_TOURNAMENTS: ReadonlySet<SeriesTab['slug']> = new Set(['international', 'intermezzo', 'royal_league', 'worlds']);

/** seriesHref returns the address of a series' page (a single tournament's own page), or of the Hall of Fame for the overview. */
export function seriesHref(slug: SeriesTab['slug']): string {
  if (slug === 'overview') return '/ratings/hall_of_fame/';
  return SINGLE_TOURNAMENTS.has(slug) ? `/ratings/tournaments/?t=${slug}` : `/ratings/hall_of_fame/${slug}/`;
}

/** A player story on a series page, as upstream's template states it. */
export interface Story {
  player: string;
  badge: string;
  title: string;
  text: string;
}

/** What a series page says about its series, copied from upstream's routes/hall_of_fame.py. */
export interface SeriesContent {
  slug: Series;
  name: string;
  subtitle: string;
  /** A circuit's card; a tournament's own card is written in admin and comes from the API. */
  icon?: string;
  formatBadge?: string;
  summary?: string;
  frequency: string;
  since: string;
  /** The "Historical Era" as upstream states it, where it is not computed from the latest season. */
  era?: string;
  /** A circuit's Format & Regulations; a tournament's are written in admin and come from the API. */
  rules?: string[];
  /** A circuit's stories; a tournament's are written in admin and come from the API. */
  stories?: Story[];
  /** A circuit of several tournaments: each, by its catalogue slug and name. */
  members?: { slug: string; name: string }[];
}

export const INTERNATIONAL: SeriesContent = {
  slug: 'international',
  name: 'International Championship',
  subtitle: '3/4-Player Competitive Format',
  frequency: 'Quarterly (4 seasons / year)',
  since: '2016',
};

export const INTERMEZZO: SeriesContent = {
  slug: 'intermezzo',
  name: 'Intermezzo Championship',
  subtitle: '3-Player Competitive Format',
  frequency: 'Quarterly (4 seasons / year)',
  since: '2018',
};

export const ROYAL_LEAGUE: SeriesContent = {
  slug: 'royal_league',
  name: 'Royal League',
  subtitle: '2-Player Competitive Format',
  frequency: 'Quarterly (4 seasons / year)',
  since: '2024',
};

export const WORLDS: SeriesContent = {
  slug: 'worlds',
  name: 'Through the Ages World Championship',
  subtitle: '4-Player Competitive Format',
  frequency: 'Annual (Every Year)',
  since: '2023',
  era: '2023 – Present (Next: 2027)',
};

export const GRAND_SLAMS: SeriesContent = {
  slug: 'grand_slams',
  icon: '🎾',
  name: 'Grand Slams & Championship Cups',
  subtitle: '4-Player Major Open Format',
  formatBadge: 'Major Cups & Slams',
  summary:
    'Prestigious open championships and multi-stage endurance cups: Survivors Cup (3P survival elimination), Wimbledon TTA (1v1 knockout duel bracket), French Open (3P/4P clay season major), Eiffel Tower Tournament (floor climbing stages to The Top), and Slow Burn.',
  frequency: 'Annual / Seasonal Majors',
  since: '2023',
  era: '2023 – Present',
  rules: [
    'Survivors Cup: 11 progressive stages of 3-player survival tables with cuts to Upper/Lower semifinals and Grand Finals.',
    'Wimbledon TTA: Direct 2-player knockout duel rounds through to Quarterfinals, Semifinals, and Finals.',
    "Eiffel Tower: Multi-level qualification climbing up the tower to 'The Top' elite division.",
    'Slow Burn: Extended multi-month endurance rounds culminating in a 3-game Finals series.',
  ],
  stories: [
    {
      player: 'Grozz',
      badge: 'Survivors Cup Champion',
      title: '2026 Survivors Cup Winner',
      text: 'Grozz conquered the 11-stage 2026 Survivors Cup, triumphing in the Grand Final (213.0 pts) against Weidenbaum (208.0 pts) and Sellux (201.0 pts).',
    },
    {
      player: 'wolvs',
      badge: 'Slow Burn Champion',
      title: 'Season 15 Slow Burn Sweep',
      text: 'wolvs swept all 3 games in the Season 15 Slow Burn Finals (218, 281, 168 pts) to claim the championship over saru and DiddleySquat.',
    },
  ],
  members: [
    { slug: 'wimbledon', name: 'Wimbledon' },
    { slug: 'french_open', name: 'French Open' },
    { slug: 'australian_open', name: 'Australian Open' },
    { slug: 'survivors_cup', name: 'Survivors Cup' },
    { slug: 'slow_burn', name: 'Slow Burn' },
    { slug: 'eiffel_tower', name: 'Eiffel Tower' },
  ],
};

export const LADDERS: SeriesContent = {
  slug: 'ladders',
  icon: '🪜',
  name: 'Competitive Ladders (Mercurial, Sodium & Transcontinental)',
  subtitle: '2 & 3-Player Continuous Ladder Format',
  formatBadge: 'Tiered Ladders',
  summary:
    'Perpetual rung-based ladders where competitors battle in 3-player and 2-player encounters to ascend through dynamic tier rankings.',
  frequency: 'Continuous / Quarterly',
  since: '2017',
  era: '2017 – Present',
  rules: [
    'Mercurial Ladder: Rolling periodic element tiers (Hydrogen, Helium ... Neon, Sodium) with performance rung promotions.',
    'Sodium Ladder: Continuous tiered metal ladders across ascending rungs.',
    'Transcontinental Ladder (TCL): 100-stage continuous challenge ladder.',
  ],
  stories: [
    {
      player: 'wolvs',
      badge: 'Ladder Legend',
      title: 'All-Time Ladder Match Leader',
      text: 'wolvs has compiled the highest match volume and victory counts across the competitive ladder circuits.',
    },
  ],
  members: [
    { slug: 'mercurial_ladder', name: 'Mercurial Ladder' },
    { slug: 'sodium_ladder', name: 'Sodium Ladder' },
    { slug: 'transcontinental_ladder', name: 'Transcontinental Ladder' },
  ],
};

/** Every series with a page, in the series bar's order. */
export const SERIES_CONTENT: SeriesContent[] = [INTERNATIONAL, INTERMEZZO, ROYAL_LEAGUE, WORLDS, GRAND_SLAMS, LADDERS];

/** seriesEra returns a series' "Historical Era": its static text, or computed from the latest season. */
export function seriesEra(content: SeriesContent, season: number): string {
  return content.era ?? era(content.since, season);
}

/** medalCells returns a board row's gold, silver, bronze and total cells; a board without medals leaves them empty, as upstream. */
export function medalCells(row: { gold: number | null; silver: number | null; bronze: number | null }): string[] {
  const { gold, silver, bronze } = row;
  if (gold === null || silver === null || bronze === null) return ['', '', '', ''];
  return [gold, silver, bronze, gold + silver + bronze].map(String);
}

type BoardRow = { gold: number | null; silver: number | null; bronze: number | null; games: number | null; wins: number | null; win_rate: number | null };

/** boardCells returns a trophyboard's column headings, their classes and cells: medals, a ladder's games, wins and win rate, or its top-tier wins alone. */
export function boardCells(rows: readonly BoardRow[]): { headings: string[]; classes: string[]; rows: string[][] } {
  if (rows.length > 0 && rows.every((r) => r.gold === null && r.games === null && r.wins !== null)) {
    return { headings: ['🏆 Top-tier wins'], classes: ['medal-gold'], rows: rows.map((r) => [String(r.wins)]) };
  }
  if (rows.some((r) => r.gold === null && r.wins !== null)) {
    const n = (x: number | null) => (x === null ? '' : x.toLocaleString('en-US'));
    return {
      headings: ['Games', 'Wins', 'Win %'],
      classes: ['', '', ''],
      rows: rows.map((r) => [n(r.games), n(r.wins), r.win_rate === null ? '' : `${r.win_rate.toFixed(1)}%`]),
    };
  }
  return {
    headings: ['🥇 Gold', '🥈 Silver', '🥉 Bronze', 'Total'],
    classes: ['medal-gold', 'medal-silver', 'medal-bronze', 'medal-total'],
    rows: rows.map(medalCells),
  };
}

/** era returns upstream's "Historical Era" for a series running since a year, at its latest season. */
export function era(since: string, season: number): string {
  return `${since} – Present (Current: Season ${season})`;
}

/** leaguePoints writes a season's points with one decimal, as upstream's "%.1f pts". */
export function leaguePoints(points: number): string {
  return `${points.toFixed(1)} pts`;
}

/** cultureScore writes a game's winning score whole, as upstream's "%.0f pts". */
export function cultureScore(score: number): string {
  return `${score.toFixed(0)} pts`;
}

/** opponentsLine lists a game's other players, a whole score without its decimal, as upstream does. */
export function opponentsLine(opponents: { name: string; score: number }[]): string {
  if (opponents.length === 0) return '';
  return `vs ${opponents.map((o) => `${o.name} (${o.score} pts)`).join(', ')}`;
}

/** seriesRecord writes a season's places, or a Royal League season's wins/losses (wins/draws/losses with draws), as upstream. */
export function seriesRecord(series: Series, placings: number[]): string {
  if (series === 'royal_league') {
    const [wins = 0, draws = 0, losses = 0] = placings;
    return draws === 0 ? `${wins}/${losses}` : `${wins}/${draws}/${losses}`;
  }
  return placings.join('/');
}

/** titleCell writes a legend's titles in one circuit: the count (with a crown for World titles), or a dash for none. */
export function titleCell(titles: number, crown = false): string {
  if (titles <= 0) return '-';
  return crown ? `${titles} 👑` : String(titles);
}

/** legendMark returns WC for a World Champion, GM for three or more major titles, else null, as upstream's overview. */
export function legendMark(l: Legend): 'WC' | 'GM' | null {
  if (l.world_champion || l.world_titles > 0) return 'WC';
  return majorTitles(l) >= 3 ? 'GM' : null;
}

/** majorTitles returns a legend's titles across the four circuits. */
export function majorTitles(l: Legend): number {
  return l.world_titles + l.international_titles + l.intermezzo_titles + l.royal_league_titles;
}

/** majorMedals writes a legend's medals and their total as upstream's overview. */
export function majorMedals(l: Legend): string {
  return `🥇${l.gold}  🥈${l.silver}  🥉${l.bronze} (${l.gold + l.silver + l.bronze})`;
}

/** derivedMatches writes the overview's match count: the four major circuits' games, as upstream sums them. */
export function derivedMatches(counts: SeriesCount[]): string {
  const major = new Set(['international', 'intermezzo', 'royal_league', 'worlds']);
  const total = counts.filter((c) => major.has(c.series)).reduce((n, c) => n + c.games, 0);
  return `${total.toLocaleString('en-US')}+ Matches`;
}

/** seriesGames writes a series card's games and editions. */
export function seriesGames(c: SeriesCount): string {
  return `${c.games.toLocaleString('en-US')} games (${c.seasons} editions)`;
}

/** showsWorldChampion says whether a World Champion's WC mark shows: only on the World Championship's own page. */
export function showsWorldChampion(series: string, champion: boolean): boolean {
  return series === 'worlds' && champion;
}
