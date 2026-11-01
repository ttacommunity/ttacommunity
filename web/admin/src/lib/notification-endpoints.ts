/** The Notifications tab's decisions: the events an endpoint may take, and whether an edit can be saved. */
import type { components } from '@ttac/shared/api';

type Event = components['schemas']['NotificationEvent'];
type Setting = { name: string; url: string; events: readonly Event[]; prefix: string };

/** EVENTS are the events an endpoint may take, in the order the tab lists them. */
export const EVENTS: { key: Event; label: string }[] = [
  { key: 'link_request', label: 'Requests to be linked' },
  { key: 'blitz_report', label: 'Blitz results reported or corrected' },
  { key: 'blitz_flag', label: 'Blitz reports flagged' },
];

/** endpointChanged says whether an endpoint's edit can be saved: it has a name and a URL, and differs from what is saved. */
export function endpointChanged(saved: Setting, draft: Setting): boolean {
  if (draft.name.trim() === '' || draft.url.trim() === '') return false;
  const sameEvents = saved.events.length === draft.events.length && saved.events.every((e) => draft.events.includes(e));
  return draft.name.trim() !== saved.name || draft.url.trim() !== saved.url || draft.prefix.trim() !== saved.prefix || !sameEvents;
}

/** toggled returns events with event taken (on) or not, in EVENTS's order. */
export function toggled(events: readonly Event[], event: Event, on: boolean): Event[] {
  return EVENTS.map((e) => e.key).filter((k) => (k === event ? on : events.includes(k)));
}
