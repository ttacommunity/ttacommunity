/** What a zone picker offers a member: Unknown and the regions the API names, then every IANA zone the browser knows. */
import type { components } from '@ttac/shared/api';
import { offsetLabel } from '@ttac/shared/blitz';
import { matchRank, norm } from './match';

export type Region = components['schemas']['Region'];
/** ZoneChoice is a zone a picker offers, with a line saying what it is ('' for an IANA zone). */
export type ZoneChoice = { value: string; detail: string };

/** knownZones is every IANA zone the browser knows, or none in one that cannot say. */
export function knownZones(): string[] {
  try {
    return Intl.supportedValuesOf('timeZone');
  } catch {
    return [];
  }
}

/** regionDetail says whose time a region keeps, "New York time, UTC-4", or that Unknown pairs as UTC. */
function regionDetail(r: Region): string {
  if (r.zone === '') return 'not known: paired as UTC';
  const city = (r.zone.split('/').pop() ?? r.zone).replaceAll('_', ' ');
  const offset = offsetLabel(r.utc_offset);
  return offset === '' ? `${city} time` : `${city} time, ${offset}`;
}

/** choices is every zone a picker can offer: Unknown and the regions, then UTC and the zones the browser knows. */
function choices(regions: readonly Region[], known: readonly string[]): { regions: ZoneChoice[]; zones: ZoneChoice[] } {
  const zones = known.includes('UTC') ? known : ['UTC', ...known];
  return { regions: regions.map((r) => ({ value: r.name, detail: regionDetail(r) })), zones: zones.map((z) => ({ value: z, detail: '' })) };
}

/** zoneChoices lists what a picker offers for the text typed: with none, Unknown and every region; else up to limit regions, then zones, that match it, each group the name itself first, then names starting with the text, holding it, and with each word typed starting one of theirs. */
export function zoneChoices(regions: readonly Region[], known: readonly string[], typed: string, limit: number): ZoneChoice[] {
  const all = choices(regions, known);
  const q = norm(typed);
  if (q === '') return all.regions;
  const rank = (c: ZoneChoice) => matchRank(c.value, q) ?? 4;
  const best = (list: ZoneChoice[]) => list.filter((c) => rank(c) < 4).sort((a, b) => rank(a) - rank(b));
  return [...best(all.regions), ...best(all.zones)].slice(0, limit);
}

/** zoneFromText is the region or zone named in full by the text, in any case, or null. */
export function zoneFromText(regions: readonly Region[], known: readonly string[], text: string): string | null {
  const all = choices(regions, known);
  const q = norm(text);
  return [...all.regions, ...all.zones].find((c) => q !== '' && norm(c.value) === q)?.value ?? null;
}

/** clockIn reads an instant on a zone's clocks (a region's city's, UTC for Unknown or a zone the browser cannot read) as a day (YYYY-MM-DD) and a time (HH:MM). */
export function clockIn(regions: readonly Region[], zone: string, instant: string): { day: string; clock: string } {
  const region = regions.find((r) => r.name === zone);
  const iana = region === undefined ? zone : region.zone === '' ? 'UTC' : region.zone;
  const read = (timeZone: string) =>
    new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(instant));
  let parts: Intl.DateTimeFormatPart[];
  try {
    parts = read(iana);
  } catch {
    parts = read('UTC');
  }
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((p) => p.type === type)?.value ?? '';
  return { day: `${part('year')}-${part('month')}-${part('day')}`, clock: `${part('hour')}:${part('minute')}` };
}
