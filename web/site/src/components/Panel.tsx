/** A panel of a page: the site's card, full width, headed by a PanelHeader (its title, an optional icon, a line under it and what it says about itself on the right), or by nothing for a panel whose parts carry their own headings. */
import type { ReactNode } from 'react';
import PanelHeader from './PanelHeader';

type Props = {
  /** The heading's id, which names the panel; leave out with title for a panel named by label. */
  id?: string;
  title?: ReactNode;
  icon?: string;
  subtitle?: ReactNode;
  /** What the panel says about itself, on the header's right: a count tag, a picker. */
  aside?: ReactNode;
  /** The panel's name for a reader, when it has no title. */
  label?: string;
  /** The page's own class for the panel's layout inside it, never its look. */
  className?: string;
  children?: ReactNode;
};

export default function Panel({ id, title, icon, subtitle, aside, label, className = '', children }: Props) {
  const named = title === undefined ? { 'aria-label': label } : { 'aria-labelledby': id };
  return (
    <section className={`rating-card panel-card ${className}`.trim()} {...named}>
      {title !== undefined && (
        <PanelHeader id={id} icon={icon} title={title} subtitle={subtitle}>
          {aside}
        </PanelHeader>
      )}
      {children}
    </section>
  );
}
