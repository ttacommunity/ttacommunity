/** Time display: every time is shown in the viewer's zone, labelled with that zone. */

const UTC_INSTANT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?Z$/;

/**
 * Formats a UTC ISO instant in an IANA zone, e.g. "1 Jul 2026, 13:00 (Europe/London)".
 * Throws RangeError for an unknown zone or an instant that is not UTC.
 */
export function formatInstant(instant: string, timeZone: string): string {
  const date = new Date(instant);
  if (!UTC_INSTANT.test(instant) || Number.isNaN(date.getTime())) {
    throw new RangeError(`not a UTC instant: ${instant}`);
  }
  const shown = new Intl.DateTimeFormat('en-GB', {
    timeZone,
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(date);
  return `${shown} (${timeZone})`;
}
