/** The audit log tab's decisions: the days it shows (the last week by default, in the viewer's zone), the query for them, who made each change and its detail as text. */
import { count } from '@ttac/shared/numbers';
import { todayIn } from '@ttac/shared/time';

/** AuditView is what the tab shows: an action ('' for all), a search, a span of days (YYYY-MM-DD, both included; '' for open) and a page. */
export type AuditView = { action: string; text: string; from: string; to: string; page: number };

const DAY_MS = 24 * 60 * 60 * 1000;

/** offsetMs returns how far zone's clocks are ahead of UTC at instant ms. */
function offsetMs(ms: number, zone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone,
    hourCycle: 'h23',
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  }).formatToParts(new Date(ms));
  const part = (type: string) => Number(parts.find((p) => p.type === type)?.value);
  return Date.UTC(part('year'), part('month') - 1, part('day'), part('hour'), part('minute'), part('second')) - ms;
}

/** dayStart returns the UTC instant (ISO) at which day (YYYY-MM-DD) begins on zone's clocks. */
export function dayStart(day: string, zone: string): string {
  const midnight = Date.parse(`${day}T00:00:00Z`);
  const guess = midnight - offsetMs(midnight, zone);
  return new Date(midnight - offsetMs(guess, zone)).toISOString();
}

/** addDays returns the day n days after day (YYYY-MM-DD). */
function addDays(day: string, n: number): string {
  return new Date(Date.parse(`${day}T00:00:00Z`) + n * DAY_MS).toISOString().slice(0, 10);
}

/** defaultView is the tab as it opens: every action, no search, the seven days to today on zone's clocks, page 1. */
export function defaultView(now: Date, zone: string): AuditView {
  const to = todayIn(zone, now, zone);
  return { action: '', text: '', from: addDays(to, -6), to, page: 1 };
}

/** isDay says whether s is a real day written YYYY-MM-DD. */
function isDay(s: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(s) && !Number.isNaN(Date.parse(`${s}T00:00:00Z`)) && addDays(s, 0) === s;
}

/** viewFromSearch reads the tab's view from its address: a missing or bad day, or a page that is not a positive whole number, is the default's; an empty day is open. */
export function viewFromSearch(search: URLSearchParams, now: Date, zone: string): AuditView {
  const base = defaultView(now, zone);
  const day = (name: 'from' | 'to') => {
    const v = search.get(name);
    return v !== null && (v === '' || isDay(v)) ? v : base[name];
  };
  const page = search.get('page') ?? '';
  return {
    action: search.get('action') ?? '',
    text: search.get('q') ?? '',
    from: day('from'),
    to: day('to'),
    page: /^[1-9]\d*$/.test(page) ? Number(page) : 1,
  };
}

/** searchOf writes view as the tab's address: its days always (empty when open), its action, search and page only when set. */
export function searchOf(view: AuditView): string {
  const q = new URLSearchParams();
  if (view.action !== '') q.set('action', view.action);
  if (view.text.trim() !== '') q.set('q', view.text.trim());
  q.set('from', view.from);
  q.set('to', view.to);
  if (view.page > 1) q.set('page', String(view.page));
  return `?${q}`;
}

/** actionChoices are the actions to choose from: the log's, with the chosen one first when the log has none of it. */
export function actionChoices(actions: string[] | undefined, chosen: string): string[] {
  const list = actions ?? [];
  return chosen === '' || list.includes(chosen) ? list : [chosen, ...list];
}

/** auditQuery returns the query asking for view, its days taken in zone, or null when its from day is after its to day. */
export function auditQuery(view: AuditView, zone: string): string | null {
  if (view.from !== '' && view.to !== '' && view.from > view.to) return null;
  const q = new URLSearchParams();
  if (view.action !== '') q.set('action', view.action);
  if (view.text.trim() !== '') q.set('q', view.text.trim());
  if (view.from !== '') q.set('from', dayStart(view.from, zone));
  if (view.to !== '') q.set('to', dayStart(addDays(view.to, 1), zone));
  q.set('page', String(view.page));
  return `?${q}`;
}

/** detailFields returns detail's fields when it is a JSON object, else null. */
function detailFields(detail: string): Record<string, unknown> | null {
  try {
    const v: unknown = JSON.parse(detail);
    return v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

/** auditBy names who made a change: the admin, a player (with their account when recorded), or someone since taken out of admin. */
export function auditBy(entry: { admin: string; by_player: boolean; detail: string }): string {
  if (entry.admin !== '') return entry.admin;
  if (!entry.by_player) return 'Someone no longer in admin';
  const account = detailFields(entry.detail)?.account;
  return typeof account === 'number' ? `A player (account ${account})` : 'A player';
}

/** detailText writes a change's detail as "field: value" pairs, empty when it has none, or as sent when it is not an object. */
export function detailText(detail: string): string {
  const fields = detailFields(detail);
  if (fields === null) return detail === 'null' ? '' : detail;
  return Object.entries(fields)
    .map(([k, v]) => `${k}: ${Array.isArray(v) ? v.join(', ') : typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)}`)
    .join(' · ');
}

/** auditSummary says how many entries match, and which page this is when there is more than one. */
export function auditSummary({ total, page, pages }: { total: number; page: number; pages: number }): string {
  if (total === 0) return 'No entries match.';
  const entries = total === 1 ? '1 entry' : `${count(total)} entries`;
  return pages > 1 ? `${entries} · page ${count(page)} of ${count(pages)}` : entries;
}
