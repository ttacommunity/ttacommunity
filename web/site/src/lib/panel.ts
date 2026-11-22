/** A Panel's look settings, and the classes that draw them. */

/** A left stripe marks a panel of commentary about the data (a summary, a guide, a finding); data panels have none. Gold: summaries and guides; green: a finding; bronze: a deep dive. */
export type PanelStripe = 'gold' | 'green' | 'bronze';

/** A panel's width, chosen by what it holds: full for tables, charts and lists; reading (centred, about 70 characters a line) for forms and prose; screen for a table with more columns than the page is wide. */
export type PanelSize = 'full' | 'reading' | 'screen';

/** What a panel holds, when it is text to read (a document, the FAQ, an explainer): drawn in the reading look. */
export type PanelHolds = 'text';

/** panelClass returns a panel's classes: the card, then its width, what it holds, its stripe and picture when set. */
export function panelClass({ size, holds, stripe, art }: { size?: PanelSize; holds?: PanelHolds; stripe?: PanelStripe; art?: string }): string {
  const classes = ['rating-card', 'panel-card'];
  if (size === 'reading' || size === 'screen') classes.push(`panel-${size}`);
  if (holds !== undefined) classes.push(`panel-${holds}`);
  if (stripe !== undefined) classes.push('panel-stripe', `panel-stripe-${stripe}`);
  if (art) classes.push('panel-art');
  return classes.join(' ');
}
