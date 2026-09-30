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

/** seriesHref returns the address of a series' page, or of the Hall of Fame for the overview. */
export function seriesHref(slug: SeriesTab['slug']): string {
  return slug === 'overview' ? '/ratings/hall_of_fame/' : `/ratings/hall_of_fame/${slug}/`;
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
  icon: string;
  name: string;
  subtitle: string;
  formatBadge: string;
  summary: string;
  frequency: string;
  since: string;
  /** The "Historical Era" as upstream states it, where it is not computed from the latest season. */
  era?: string;
  rules: string[];
  stories: Story[];
}

export const INTERNATIONAL: SeriesContent = {
  slug: 'international',
  icon: '🌐',
  name: 'International Championship',
  subtitle: '3/4-Player Competitive Format',
  formatBadge: '3/4-Player League',
  summary:
    'The premier 3/4-player competitive league in Through the Ages. Structured across hierarchical skill divisions (Grandmaster, Platinum, Gold, Silver, Bronze, Wood) with quarterly promotion and relegation.',
  frequency: 'Quarterly (4 seasons / year)',
  since: '2016',
  rules: [
    '7 players per division playing a standard seasonal schedule of 7 matches (4 four-player games and 3 three-player games).',
    'Scoring System: 4-Player games award 6/3/1/0 points; 3-Player games award 5/2/0 points (ties split points equally).',
    'Theoretical Seasonal Maximum: (4 x 6) + (3 x 5) = 39.0 points.',
    'Divisional pyramid: Grandmaster, Master (1 & 2), Platinum (1-4), Gold (1-8), Silver (1-14), Bronze (1-14), Wood (1-10).',
    'Top division crowns the official International Champion each season.',
  ],
  stories: [
    {
      player: 'Weidenbaum',
      badge: 'Dynastic Record',
      title: 'The Golden Hexa-Crown • 6 International Titles & 9 Podiums',
      text: 'Grandmaster Weidenbaum holds the all-time championship record in the International Championship with 6 GM Division titles, 2 runner-up finishes, and 1 bronze across 14 competitive seasons.',
    },
    {
      player: 'Genghisip',
      badge: 'Podium Benchmark',
      title: 'All-Time Medal Leader • 11 Top-3 Finishes & 4 Titles',
      text: 'Holding the all-time record for total podium finishes in the International Championship, Genghisip captured 4 Gold medals, 2 Silvers, and 5 Bronzes across 15 recorded seasons.',
    },
    {
      player: 'DANeo',
      badge: 'Record Campaign',
      title: 'All-Time Scoring Season • 36.0 / 39.0 Points & Premier Title',
      text: 'DANeo authored one of the most dominant seasonal campaigns in premier 4-player history during Season 31, scoring 36.0 out of a maximum 39.0 points in the Grandmaster Division to claim the Gold crown.',
    },
  ],
};

export const INTERMEZZO: SeriesContent = {
  slug: 'intermezzo',
  icon: '⚔️',
  name: 'Intermezzo Championship',
  subtitle: '3-Player Competitive Format',
  formatBadge: '3-Player League',
  summary: 'The premier 3-player competitive league.',
  frequency: 'Quarterly (4 seasons / year)',
  since: '2018',
  rules: [
    '5 players per division in a round-robin schedule totaling 10 matches per division.',
    'Each player competes in exactly 6 games per season (3-player tables paired with every pair of division rivals).',
    'Scoring System: 1st Place = 5 pts, 2nd Place = 2 pts, 3rd Place = 0 pts (ties split points equally).',
    'Theoretical Seasonal Maximum: 6 games x 5.0 pts = 30.0 points.',
    'Multi-divisional ladder structure with Master 1 / Diamond as the highest tier.',
  ],
  stories: [
    {
      player: 'Weidenbaum',
      badge: 'All-Time Dynasty',
      title: 'Eight-Time Champion • 15 Premier Podiums',
      text: 'Weidenbaum stands as the most decorated champion in Intermezzo history, claiming 8 Gold medals, 3 Silver medals, and 4 Bronze medals across 17 seasons, totaling an unmatched 314 Hall of Fame points.',
    },
    {
      player: 'Martin_Pecheur',
      badge: 'Triple Champion',
      title: 'World Champion Pedigree • 3 Titles & 8 Podiums',
      text: 'Martin_Pecheur captured 3 Intermezzo Championships, 3 runner-up finishes, and 2 bronze medals across 10 seasons (172 Hall of Fame points).',
    },
  ],
};

export const ROYAL_LEAGUE: SeriesContent = {
  slug: 'royal_league',
  icon: '👑',
  name: 'Royal League',
  subtitle: '2-Player Competitive Format',
  formatBadge: '2-Player Duel',
  summary: 'The premier 2-player recurrent championship.',
  frequency: 'Quarterly (4 seasons / year)',
  since: '2024',
  rules: [
    '2-player head-to-head matches exclusively under medium async with 48h reserve per Age.',
    '8 players per division playing 7 weekly match rounds against all group opponents (28 matches total; max 14.0 pts).',
    'Points: 2 for win, 1 for draw, 0 for loss.',
    'Tiebreaker Protocol: 1st tiebreaker is Head-to-Head (H2H) match results between tied players. If unresolved for critical promotion, relegation, or podium finishes, official staged playoff tiebreaker tournaments are run.',
    'Fibonacci-pyramidal divisional hierarchy: Emperor, King, Prince, Duke, Marquess, Count, Viscount, Baron.',
  ],
  stories: [
    {
      player: 'DANeo',
      badge: 'Reigning Emperor Champion',
      title: 'Season 9 Emperor Gold & Multi-Podium Legend',
      text: 'DANeo captured the Season 9 Royal League Emperor Division Championship with a 6W-1L (12.0 pts) performance, alongside two Silver Medals in Season 4 and Season 7.',
    },
    {
      player: 'saru',
      badge: 'Two-Time Emperor Champion',
      title: 'Back-to-Back Titles & Season 9 Silver',
      text: 'saru captured back-to-back Royal League Premier Emperor Division titles in Season 7 and Season 8, followed by a Silver finish in Season 9 (clinching on H2H tiebreaker).',
    },
    {
      player: 'vanishadow',
      badge: 'Inaugural Dynasty',
      title: 'Inaugural Back-to-Back Champion (Season 1 & 2)',
      text: 'vanishadow claimed the first two Royal League championships in history, capturing Gold in Season 1 and Season 2, followed by a Silver finish in Season 3.',
    },
  ],
};

export const WORLDS: SeriesContent = {
  slug: 'worlds',
  icon: '🏆',
  name: 'Through the Ages World Championship',
  subtitle: '4-Player Competitive Format',
  formatBadge: 'Annual World Championship',
  summary:
    'The pinnacle of competitive Through the Ages. An annual championship bringing together the top qualified competitors worldwide across multi-stage preliminary groups, playoff brackets, and championship finals. (Note: historical format regulations have evolved across editions, and future editions may introduce new architectures).',
  frequency: 'Annual (Every Year)',
  since: '2023',
  era: '2023 – Present (Next: 2027)',
  rules: [
    'Annual world championship staged across multi-round qualifying and playoff brackets.',
    'Historically features progressive group stages (Stage 1 & 2) and upper/lower elimination playoff brackets (Stage 3 & 4).',
    'Format regulations evolve over time with special structures planned for upcoming editions.',
    'Winner receives the permanent World Champion title badge and exclusive Crown WC distinction.',
  ],
  stories: [
    {
      player: 'a440',
      badge: 'Reigning World Champion',
      title: 'The 2025 World Championship Title',
      text: 'a440 captured the 2025 Through the Ages World Championship, surviving the grueling multi-stage qualification and navigating the Stage 4 Upper playoff bracket to claim the world crown.',
    },
    {
      player: 'Martin_Pecheur',
      badge: '2024 World Champion',
      title: 'Playoff Sweep & World Championship Crown',
      text: 'Martin_Pecheur swept the 2024 World Championship playoff brackets, defeating top grandmasters in the championship finals.',
    },
    {
      player: 'Weidenbaum',
      badge: 'Inaugural World Champion',
      title: 'The Inaugural 2023 World Championship',
      text: 'Weidenbaum was crowned the inaugural Through the Ages World Champion in 2023, adding the definitive global title to his record 6 International and 8 Intermezzo league championships.',
    },
  ],
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
