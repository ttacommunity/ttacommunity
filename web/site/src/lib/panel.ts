/** A Panel's look settings, and the classes that draw them. */

/** A left stripe marks a panel of commentary about the data (a summary, a guide, a finding); data panels have none. Gold: summaries and guides; green: a finding; bronze: a deep dive. */
export type PanelStripe = 'gold' | 'green' | 'bronze';

/** A panel's width, chosen by what it holds: full for tables, charts and lists; reading (centred, about 70 characters a line) for forms and prose. */
export type PanelSize = 'full' | 'reading';

/** panelClass returns a panel's classes: the card, then its width, stripe and picture when set, then the page's own layout class. */
export function panelClass({ size, stripe, art, className }: { size?: PanelSize; stripe?: PanelStripe; art?: string; className?: string }): string {
  const classes = ['rating-card', 'panel-card'];
  if (size === 'reading') classes.push('panel-reading');
  if (stripe !== undefined) classes.push('panel-stripe', `panel-stripe-${stripe}`);
  if (art) classes.push('panel-art');
  if (className) classes.push(className);
  return classes.join(' ');
}
