/** What both sites say about a blitz tournament: a zone's offset, a format, and the zones a select offers. */
import type { components } from '../api/schema';

export type BlitzFormat = components['schemas']['BlitzFormat'];

/** offsetLabel shows an offset the API sends (+02:00) as UTC+2, UTC-4 or UTC+5:30; UTC for none, '' for unknown. */
export function offsetLabel(offset: string): string {
  const m = /^([+-])(\d\d):(\d\d)$/.exec(offset);
  if (m === null) return '';
  const [, sign, hours, minutes] = m;
  const h = Number(hours);
  if (h === 0 && minutes === '00') return 'UTC';
  return `UTC${sign}${h}${minutes === '00' ? '' : `:${minutes}`}`;
}

/** zoneLabel shows a player's zone with its offset now, as "UTC+2 · Europe/Berlin", or the zone alone. */
export function zoneLabel(zone: string, offset: string): string {
  const o = offsetLabel(offset);
  return o === '' ? zone : `${o} · ${zone}`;
}

/** formatLabel describes an edition's format in words. */
export function formatLabel(f: BlitzFormat): string {
  const swiss = `${f.swiss_rounds} Swiss round${f.swiss_rounds === 1 ? '' : 's'}`;
  return f.knockout_size === 0 ? `${swiss}, no knockout` : `${swiss}, then a top-${f.knockout_size} knockout`;
}

/** zoneOptions lists the zones a select offers: UTC first, then known in order, with current added when missing. */
export function zoneOptions(known: readonly string[], current: string): string[] {
  const rest = known.filter((z) => z !== 'UTC');
  if (current !== '' && current !== 'UTC' && !rest.includes(current)) rest.push(current);
  return ['UTC', ...rest.sort()];
}
