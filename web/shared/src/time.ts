/** Time display: every time is shown in the viewer's zone, labelled with that zone. */

const UTC_INSTANT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?Z$/;

/**
 * Formats a UTC ISO instant in an IANA zone, e.g. "1 Jul 2026, 13:00 (Europe/London)".
 * Throws RangeError for an unknown zone or an instant that is not UTC.
 */
export function formatInstant(instant: string, timeZone: string): string {
  return `${formatTime(instant, timeZone)} (${timeZone})`;
}

/** Formats a UTC ISO instant in an IANA zone without naming it, e.g. "1 Jul 2026, 13:00", for a view that names the zone once. Throws as formatInstant does. */
export function formatTime(instant: string, timeZone: string): string {
  const date = new Date(instant);
  if (!UTC_INSTANT.test(instant) || Number.isNaN(date.getTime())) {
    throw new RangeError(`not a UTC instant: ${instant}`);
  }
  return new Intl.DateTimeFormat('en-GB', {
    timeZone,
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(date);
}

/** formatDay writes a calendar date (YYYY-MM-DD) as "24 Feb 2025", the same day in every time zone. */
export function formatDay(day: string): string {
  return new Date(`${day}T00:00:00Z`).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' });
}

/** viewerZone returns the browser's IANA time zone, the one a visitor's times are shown in. */
export function viewerZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** formatDate writes the day an instant falls on in a time zone, such as "24 Feb 2025". */
export function formatDate(instant: string, timeZone: string): string {
  return new Date(instant).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone });
}

const CLOCK = { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' } as const;

/** readClock reads an instant with a formatter of CLOCK's fields as a day (YYYY-MM-DD) and a time (HH:MM). */
function readClock(format: Intl.DateTimeFormat, instant: string): { day: string; clock: string } {
  const parts = Object.fromEntries(format.formatToParts(new Date(instant)).map((p) => [p.type, p.value]));
  return { day: `${parts.year}-${parts.month}-${parts.day}`, clock: `${parts.hour}:${parts.minute}` };
}

/** clockIn reads an instant on an IANA zone's clocks as a day (YYYY-MM-DD) and a time (HH:MM), or null for a zone the browser cannot read (a region, Unknown). */
export function clockIn(zone: string, instant: string): { day: string; clock: string } | null {
  let format: Intl.DateTimeFormat;
  try {
    format = new Intl.DateTimeFormat('en-CA', { timeZone: zone, ...CLOCK });
  } catch {
    return null;
  }
  return readClock(format, instant);
}

/** clockOrUtc reads an instant as clockIn does, on UTC's clocks for a zone the browser cannot read. */
export function clockOrUtc(zone: string, instant: string): { day: string; clock: string } {
  return clockIn(zone, instant) ?? readClock(new Intl.DateTimeFormat('en-CA', { timeZone: 'UTC', ...CLOCK }), instant);
}

/** shownZone is the zone to show times in: the saved zone when the browser can read it, else the browser's. */
export function shownZone(saved: string, browser: string): string {
  return clockIn(saved, new Date(0).toISOString()) === null ? browser : saved;
}

/** knownZones is every IANA zone the browser knows, or none in one that cannot say. */
export function knownZones(): string[] {
  try {
    return Intl.supportedValuesOf('timeZone');
  } catch {
    return [];
  }
}

/** todayIn returns the day (YYYY-MM-DD) it is at now on zone's clocks; a zone the browser cannot read (a region, Unknown) takes browser's. */
export function todayIn(zone: string, now: Date, browser = viewerZone()): string {
  const at = now.toISOString();
  return (clockIn(zone, at) ?? clockOrUtc(browser, at)).day;
}
