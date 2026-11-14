/** What a player's header says of their title: the one they hold now, and their peak with its year. */
import { titleName, type Player } from '@ttac/shared/ratings';
import { titleText } from './leaderboard-display';

/** headerTitle is the title a player holds now, named in full ("Reigning World Champion" for the one champion), or null; a past world title is their peak, never this. */
export function headerTitle(title: Player['title']): { code: Exclude<Player['title'], null>['code']; text: string; hover: string } | null {
  if (title === null) return null;
  const name = title.code === 'WC' ? 'Reigning World Champion' : titleName(title.code);
  return { code: title.code, text: title.code === 'WC' ? `💎 ${name}` : name, hover: title.reason || name };
}

/** headerPeak is the highest title a player has reached, with its year ("Career" when it has none), or null. */
export function headerPeak(peak: Player['peak']): { code: Exclude<Player['peak'], null>['code']; text: string } | null {
  if (peak === null) return null;
  return { code: peak.code, text: `${titleText(peak.code, true)} (${peak.year ?? 'Career'})` };
}
