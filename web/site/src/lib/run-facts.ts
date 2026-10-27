// What the footer says about the data: from the current run, as the engine recorded it, and the site's visits.
import type { Run, SiteVisits } from '@ttac/shared/ratings';
import { count } from '@ttac/shared/numbers';
import { formatInstant } from '@ttac/shared/time';
import { runYears } from './player-view';

/** runFacts returns the footer's text about a run and the site's visits, each part only when recorded, timed in timeZone. */
export function runFacts(run: Run, timeZone: string, visits?: SiteVisits | null): string {
  const parts: string[] = [];
  const years = runYears(run.first_game, run.last_game);
  if (run.games !== null) parts.push(`${count(run.games)} games`);
  if (run.players !== null) parts.push(`${count(run.players)} players`);
  if (visits) parts.push(`${count(visits.visits)} visits (last ${visits.days} days)`);
  if (run.finished_at !== null) parts.push(`updated ${formatInstant(run.finished_at, timeZone)}`);
  const span = years ? ` (${years[0]}–${years[1]})` : '';
  return parts.length === 0 && span === '' ? '' : span + parts.map((p) => ` • ${p}`).join('');
}
