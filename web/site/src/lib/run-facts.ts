// What the footer says about the data: from the current run, as the engine recorded it.
import type { Run } from '@ttac/shared/ratings';
import { formatInstant } from '@ttac/shared/time';
import { runYears } from './player-view';

/** runFacts returns the footer's text about a run, each part only when the run recorded it, timed in timeZone. */
export function runFacts(run: Run, timeZone: string): string {
  const parts: string[] = [];
  const years = runYears(run.first_game, run.last_game);
  const count = (n: number, noun: string) => `${n.toLocaleString('en-US')} ${noun}`;
  if (run.games !== null) parts.push(count(run.games, 'games'));
  if (run.players !== null) parts.push(count(run.players, 'players'));
  if (run.finished_at !== null) parts.push(`updated ${formatInstant(run.finished_at, timeZone)}`);
  const span = years ? ` (${years[0]}–${years[1]})` : '';
  return parts.length === 0 && span === '' ? '' : span + parts.map((p) => ` • ${p}`).join('');
}
