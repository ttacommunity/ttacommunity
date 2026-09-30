/** Upstream's Hall of Fame series pages: the series tabs, each series' text, and how its figures are written. */
import type { Series } from '@ttac/shared/ratings';

/** A tab in the series bar: the Hall of Fame overview or one series. */
export interface SeriesTab {
  slug: 'overview' | Series | 'intermezzo' | 'royal_league' | 'worlds' | 'grand_slams' | 'ladders';
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
export const BUILT: ReadonlySet<SeriesTab['slug']> = new Set(['international']);

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

/** seriesRecord writes an International season's 1st, 2nd, 3rd and 4th places. */
export function seriesRecord(placings: number[]): string {
  return placings.join('/');
}
