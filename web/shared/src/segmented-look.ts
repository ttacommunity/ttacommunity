/** Segmented's three looks, and the classes that draw a group and its buttons. */

/** choice: a setting of what is shown (the format bar); tabs: which part of a page (the records tabs); filter: which of many, such as a year (the year pills). */
export type Look = 'choice' | 'tabs' | 'filter';

const GROUP: Record<Look, string> = { choice: 'format-bar-group', tabs: 'ach-tabs', filter: 'year-pills' };
const BUTTON: Record<Look, string> = { choice: 'format-btn', tabs: 'records-tab-btn', filter: 'year-pill' };

/** groupClass returns a group's classes in the look asked for; stacked puts one option on each line, for a narrow column such as the options drawer; fill shares a bar's width among the options on a phone, two to a row. */
export function groupClass(look: Look, stacked: boolean, fill = false): string {
  return [GROUP[look], stacked ? 'stacked' : '', fill ? 'fill' : ''].filter((c) => c !== '').join(' ');
}

/** buttonClass returns an option's classes in the look asked for, pressed when chosen. */
export function buttonClass(look: Look, chosen: boolean): string {
  return chosen ? `${BUTTON[look]} active` : BUTTON[look];
}
