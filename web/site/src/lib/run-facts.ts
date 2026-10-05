// What the footer says about the data: from the current run, as the engine recorded it.
import type { Run } from '@ttac/shared/ratings';
import { runYears } from './player-view';

/** runFacts returns the footer's text about a run, each part only when the run recorded it, dated in timeZone. */
export function runFacts(run: Run, timeZone?: string): string {
  const parts: string[] = [];
  const years = runYears(run.first_game, run.last_game);
  const count = (n: number, noun: string) => `${n.toLocaleString('en-US')} ${noun}`;
  if (run.games !== null) parts.push(count(run.games, 'games'));
  if (run.players !== null) parts.push(count(run.players, 'players'));
  if (run.finished_at !== null) {
    const day = new Date(run.finished_at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone });
    parts.push(`updated ${day}`);
  }
  const span = years ? ` (${years[0]}–${years[1]})` : '';
  return parts.length === 0 && span === '' ? '' : span + parts.map((p) => ` • ${p}`).join('');
}
