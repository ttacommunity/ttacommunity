/** A status's tones (the shared .status label): muted by default, or gold, good, warn or bad. */

export type StatusTone = 'muted' | 'gold' | 'good' | 'warn' | 'bad';

/** statusClass returns the classes that draw a status in tone. */
export function statusClass(tone: StatusTone): string {
  return tone === 'muted' ? 'status' : `status status-${tone}`;
}
