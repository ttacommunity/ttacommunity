/** What admin decides about a round's Discord threads: whether they can be downloaded yet, the file's name, the round's deadline as a form and a label, when the edition's thread text can be saved, and the summaries of its organisers and text. */
import type { Candidate } from './person-form';
import { clockIn, type Region } from './zones';

/** BlitzDeadline is a round's deadline: the instant, and the zone it was set in. */
export type BlitzDeadline = { at: string; zone: string };

/** threadsBlocker says why a round's Discord threads cannot be downloaded yet, '' when they can: changes not saved, no deadline, no organiser, or no game. The API refuses the last three too. */
export function threadsBlocker(round: { deadline: BlitzDeadline | null; matches: readonly { b: unknown }[] }, organisers: number, unsaved: boolean): string {
  if (unsaved) return 'Save or discard the changes first.';
  if (round.deadline === null) return 'Set the deadline first.';
  if (organisers === 0) return 'Add the organisers first, under Organisers.';
  if (!round.matches.some((m) => m.b !== null)) return 'No games to make threads for.';
  return '';
}

/** threadsFileName names a round's threads download as the API does: us_open-2026-round-3-threads.txt. */
export function threadsFileName(slug: string, edition: string, round: number): string {
  return `${slug}-${edition.replace(/[^A-Za-z0-9_-]+/g, '-')}-round-${round}-threads.txt`;
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

/** OrganiserChoice is someone the organiser search offers: a member to pick, or a player who is no member, not to be picked. */
export type OrganiserChoice = { key: string; label: string; detail: string; disabled: boolean };

/** organiserChoices offers what a search found as organisers: each member by name with their Discord username, a player who is no member yet as one to add under Members first, and nobody who organises the edition already. */
export function organiserChoices(found: readonly Candidate[], organisers: readonly { member: number }[]): OrganiserChoice[] {
  return found
    .filter((c) => c.member === null || !organisers.some((o) => o.member === c.member))
    .map((c) =>
      c.member === null
        ? { key: `player-${c.name}`, label: c.name, detail: 'not a member: add them under Members first', disabled: true }
        : { key: String(c.member), label: c.name, detail: c.discord_name === '' ? 'no Discord username yet' : `@${c.discord_name}`, disabled: false },
    );
}

/** deadlineLocked says whether a round's deadline no longer changes: every game has a result (byes aside), as the API refuses. A round with no game is not finished. */
export function deadlineLocked(round: { matches: readonly { b: unknown; result: unknown }[] }): boolean {
  const games = round.matches.filter((m) => m.b !== null);
  return games.length > 0 && games.every((m) => m.result !== null);
}

/** ThreadText is an edition's thread text: the welcome message and the sentence for a player without Discord. */
export type ThreadText = { message: string; no_discord: string };

/** threadTextSavable says whether the thread text form can be saved: both parts written, and one changed from the saved text once trimmed, as the API trims it. The API checks the placeholders. */
export function threadTextSavable(form: ThreadText, saved: ThreadText): boolean {
  const message = form.message.trim();
  const noDiscord = form.no_discord.trim();
  return message !== '' && noDiscord !== '' && (message !== saved.message || noDiscord !== saved.no_discord);
}

/** organisersSummary sums up the edition's organisers while they are hidden: their names in the order added, or None yet. */
export function organisersSummary(organisers: readonly { name: string }[]): string {
  return organisers.length === 0 ? 'None yet' : organisers.map((o) => o.name).join(', ');
}

/** threadTextSummary sums up the edition's thread text while it is hidden: the default, or the edition's own. */
export function threadTextSummary(text: { edited: boolean }): string {
  return text.edited ? "This edition's own text" : 'The default text';
}
