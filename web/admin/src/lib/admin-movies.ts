import type { components } from '@ttac/shared/api';

export type AdminMoviesState = components['schemas']['AdminMoviesState'];
export type AdminVideoChannel = components['schemas']['AdminVideoChannel'];
export type AdminVideo = components['schemas']['AdminVideo'];

/** Validates and normalizes the Add Creator form fields before sending to the API. */
export function buildNewChannelBody(channel: string, name: string, player: string, includeAll: boolean): {
  ok: true;
  body: components['schemas']['NewVideoChannel'];
} | {
  ok: false;
  message: string;
} {
  const c = channel.trim();
  const n = name.trim();
  const p = player.trim();
  if (!c) return { ok: false, message: 'Enter a YouTube channel URL, @handle, or UC... ID.' };
  if (!n) return { ok: false, message: 'Enter a creator display name.' };
  return {
    ok: true,
    body: {
      channel: c,
      name: n,
      ...(p ? { player: p } : {}),
      include_all: includeAll,
    },
  };
}

/** An admin's say on a video: leave it to its verdict, show it, or keep it off the Movies page. */
export type OverrideChoice = 'verdict' | 'show' | 'hide';

/** The choice a video's override stands for. */
export function overrideChoice(v: AdminVideo): OverrideChoice {
  if (v.tta_override === null || v.tta_override === undefined) return 'verdict';
  return v.tta_override ? 'show' : 'hide';
}

/** The override the API takes for a choice. */
export function overrideOf(choice: OverrideChoice): boolean | null {
  return choice === 'verdict' ? null : choice === 'show';
}

/** Whether the Movies page shows a video, and why: its verdict's reason, or an admin's say. */
export function videoStatus(v: AdminVideo): { shown: boolean; why: string } {
  return { shown: v.shown, why: overrideChoice(v) === 'verdict' ? v.tta_reason : `by an admin (${v.tta_reason})` };
}

/** A channel's subscription to YouTube's pushes, as admin shows it: waiting, ok, or failing and why, at now. The
 * daily job clears an error when a run goes well, so one recorded always shows. Its times are written with when. */
export function channelStatus(
  c: AdminVideoChannel,
  now: Date,
  when: (iso: string) => string,
): { kind: 'waiting' | 'ok' | 'failing'; text: string } {
  if (c.last_error && c.last_error_at) {
    return { kind: 'failing', text: `Failing since ${when(c.last_error_at)}: ${c.last_error}` };
  }
  if (!c.subscribed_at || !c.lease_ends_at) return { kind: 'waiting', text: 'Not subscribed yet' };
  if (new Date(c.lease_ends_at) <= now) return { kind: 'failing', text: `Subscription ended ${when(c.lease_ends_at)}` };
  const pushed = c.last_push_at ? `; last video ${when(c.last_push_at)}` : '';
  return { kind: 'ok', text: `Subscribed until ${when(c.lease_ends_at)}${pushed}` };
}

/** What admin says when it starts the Movies job for the creator name, or every creator when null. */
export function moviesRunStarted(name: string | null): string {
  return name === null
    ? 'Started the Movies job for every creator. Each row shows how it went when the job ends, in a minute or two.'
    : `Started the Movies job for ${name}. Its row shows how it went when the job ends, in a minute or two.`;
}
