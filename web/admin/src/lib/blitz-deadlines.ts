/** What admin decides about a round's deadline and its Discord text: whether the text can be shown yet, and the deadline as a form and a label. */
import { clockIn, type Region } from './zones';

/** BlitzDeadline is a round's deadline: the instant, and the zone it was set in. */
export type BlitzDeadline = { at: string; zone: string };

/** discordBlocker says why a round's Discord text cannot be shown yet, '' when it can: changes not saved, or no game. The API refuses the last too. */
export function discordBlocker(round: { matches: readonly { b: unknown }[] }, unsaved: boolean): string {
  if (unsaved) return 'Save or discard the changes first.';
  if (!round.matches.some((m) => m.b !== null)) return 'No games to write.';
  return '';
}

/** DeadlineForm is the deadline form's fields: a day (YYYY-MM-DD) and a time (HH:MM) on the clocks of zone. */
export type DeadlineForm = { day: string; time: string; zone: string };

/** deadlineForm fills the deadline form from a round's deadline, on the clocks of the zone it was set in; with none, empty in zone. */
export function deadlineForm(regions: readonly Region[], deadline: BlitzDeadline | null, zone: string): DeadlineForm {
  if (deadline === null) return { day: '', time: '', zone };
  const { day, clock } = clockIn(regions, deadline.zone, deadline.at);
  return { day, time: clock, zone: deadline.zone };
}

/** deadlineLabel shows a round's deadline as it was set: "deadline 2026-10-19 13:00 Europe/London", or "no deadline". */
export function deadlineLabel(regions: readonly Region[], deadline: BlitzDeadline | null): string {
  if (deadline === null) return 'no deadline';
  const { day, clock } = clockIn(regions, deadline.zone, deadline.at);
  return `deadline ${day} ${clock} ${deadline.zone}`;
}

/** deadlineLocked says whether a round's deadline no longer changes: every game has a result (byes aside), as the API refuses. A round with no game is not finished. */
export function deadlineLocked(round: { matches: readonly { b: unknown; result: unknown }[] }): boolean {
  const games = round.matches.filter((m) => m.b !== null);
  return games.length > 0 && games.every((m) => m.result !== null);
}
