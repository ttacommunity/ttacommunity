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

/** DatePreset is the Published filter's choice: any time, the last 7 or 30 days, the last 3 or 12 months, or from and to. */
export type DatePreset = 'any' | '7d' | '30d' | '3m' | '12m' | 'custom';

/** DATE_PRESETS are the Published filter's choices as admin lists them. */
export const DATE_PRESETS: readonly { value: DatePreset; label: string }[] = [
  { value: 'any', label: 'Any time' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
  { value: '3m', label: 'Last 3 months' },
  { value: '12m', label: 'Last 12 months' },
  { value: 'custom', label: 'From – to' },
];

/** ReasonKind groups the TTA check's reasons: every video of the channel, a mention of TTA, a tournament named,
 * no mention, or another. */
export type ReasonKind = 'every' | 'mentions' | 'tournament' | 'none' | 'other';

/** REASONS are the Reason filter's choices. */
export const REASONS: readonly { value: ReasonKind; label: string }[] = [
  { value: 'mentions', label: 'Mentions TTA' },
  { value: 'tournament', label: 'Names a tournament' },
  { value: 'every', label: 'Every video counts' },
  { value: 'none', label: 'No mention of TTA' },
  { value: 'other', label: 'Other' },
];

/** reasonKind says which kind of reason the TTA check's reason is. */
export function reasonKind(reason: string): ReasonKind {
  if (reason === "the channel's every video counts") return 'every';
  if (reason.startsWith('mentions ')) return 'mentions';
  if (reason.startsWith('names the tournament ')) return 'tournament';
  if (reason === 'no mention of TTA') return 'none';
  return 'other';
}

/** STATUSES are the Status filter's choices. */
export const STATUSES: readonly { value: 'shown' | 'hidden'; label: string }[] = [
  { value: 'shown', label: 'Shown' },
  { value: 'hidden', label: 'Not shown' },
];

/** SHOWN_BY are the Shown by filter's choices. */
export const SHOWN_BY: readonly { value: OverrideChoice; label: string }[] = [
  { value: 'verdict', label: 'Its verdict' },
  { value: 'show', label: 'Always show' },
  { value: 'hide', label: 'Never show' },
];

/** VideoFilters is what admin's filter bar narrows the videos to; an empty list of choices means any. */
export type VideoFilters = {
  title: string;
  creators: readonly string[];
  date: DatePreset;
  from: string;
  to: string;
  statuses: readonly ('shown' | 'hidden')[];
  reasons: readonly ReasonKind[];
  shownBy: readonly OverrideChoice[];
};

/** NO_VIDEO_FILTERS shows every video. */
export const NO_VIDEO_FILTERS: VideoFilters = { title: '', creators: [], date: 'any', from: '', to: '', statuses: [], reasons: [], shownBy: [] };

/** firstDay returns the first day (YYYY-MM-DD) a preset takes in, counting back from today; '' for none. */
function firstDay(preset: DatePreset, today: string): string {
  const d = new Date(`${today}T00:00:00Z`);
  switch (preset) {
    case '7d':
      d.setUTCDate(d.getUTCDate() - 6);
      break;
    case '30d':
      d.setUTCDate(d.getUTCDate() - 29);
      break;
    case '3m':
      d.setUTCMonth(d.getUTCMonth() - 3);
      break;
    case '12m':
      d.setUTCMonth(d.getUTCMonth() - 12);
      break;
    default:
      return '';
  }
  return d.toISOString().slice(0, 10);
}

/** videosMatching keeps, in their order, the videos every filter takes: the title holding the text in any case,
 * a creator ticked, published within the dates (dayOf reads an instant as the viewer's day; today is theirs),
 * and a status, reason and who decides ticked. */
export function videosMatching(videos: readonly AdminVideo[], f: VideoFilters, dayOf: (iso: string) => string, today: string): AdminVideo[] {
  const text = f.title.trim().toLowerCase();
  const from = f.date === 'custom' ? f.from : firstDay(f.date, today);
  const to = f.date === 'custom' ? f.to : '';
  const any = <T,>(ticked: readonly T[], value: T) => ticked.length === 0 || ticked.includes(value);
  return videos.filter((v) => {
    const day = dayOf(v.published_at);
    return (
      v.title.toLowerCase().includes(text) &&
      any(f.creators, v.channel_id) &&
      (from === '' || day >= from) &&
      (to === '' || day <= to) &&
      any(f.statuses, v.shown ? 'shown' : 'hidden') &&
      any(f.reasons, reasonKind(v.tta_reason)) &&
      any(f.shownBy, overrideChoice(v))
    );
  });
}

/** VIDEOS_PER_PAGE is how many videos admin's table shows at once. */
export const VIDEOS_PER_PAGE = 50;

/** videoPage returns page of videos, VIDEOS_PER_PAGE a page, moved into range when the list is shorter or longer,
 * with the page it is and how many there are (one, empty, for no videos). */
export function videoPage(videos: readonly AdminVideo[], page: number): { page: number; pages: number; rows: AdminVideo[] } {
  const pages = Math.max(1, Math.ceil(videos.length / VIDEOS_PER_PAGE));
  const at = Math.min(Math.max(1, page), pages);
  return { page: at, pages, rows: videos.slice((at - 1) * VIDEOS_PER_PAGE, at * VIDEOS_PER_PAGE) };
}
