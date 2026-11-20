/** A panel of a page: the site's card, full width, headed by a PanelHeader (its title, an optional icon, a line under it and what it says about itself on the right), or by nothing for a panel whose parts carry their own headings. */
import { useId, type ReactNode, type Ref } from 'react';
import PanelHeader from './PanelHeader';
import { panelClass, type PanelStripe } from '../lib/panel';

type Props = {
  /** The heading's id, for a link to the panel; one is made when left out. */
  id?: string;
  title?: ReactNode;
  icon?: string;
  subtitle?: ReactNode;
  /** What the panel says about itself, on the header's right: a count tag, a picker. */
  aside?: ReactNode;
  /** The panel's name for a reader, when it has no title. */
  label?: string;
  /** A left stripe, only on a panel of commentary about the data: a summary, a guide, a finding. */
  stripe?: PanelStripe;
  /** A picture shown faintly behind the panel, such as a challenge group's art. */
  art?: string;
  /** The page's own class for the panel's layout inside it, never its look. */
  className?: string;
  children?: ReactNode;
  /** The panel's element, for a page that scrolls to it. */
  ref?: Ref<HTMLElement>;
};

export default function Panel({ id, title, icon, subtitle, aside, label, stripe, art, className, children, ref }: Props) {
  const ownId = useId();
  const headingId = id ?? ownId;
  const named = title === undefined ? { 'aria-label': label } : { 'aria-labelledby': headingId };
  const body = (
    <>
      {title !== undefined && (
        <PanelHeader id={headingId} icon={icon} title={title} subtitle={subtitle}>
          {aside}
        </PanelHeader>
      )}
      {children}
    </>
  );
  return (
    <section ref={ref} className={panelClass({ stripe, art, className })} {...named}>
      {art ? (
        <>
          <img className="panel-art-picture" src={art} alt="" loading="lazy" />
          <div className="panel-art-body">{body}</div>
        </>
      ) : (
        body
      )}
    </section>
  );
}
