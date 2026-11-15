/** A page's title card, the same on both sites: an optional back link and line above, the title with its icon, a subtitle, and the page's own extras. */
import type { ReactNode } from 'react';
import BackLink from './BackLink';

interface Props {
  title: ReactNode;
  /** An emoji before the title. */
  icon?: string;
  subtitle?: ReactNode;
  back?: { href: string; label: string };
  /** A line above the title, such as where the page sits. */
  above?: ReactNode;
  /** A picture beside the text, such as a challenge's card. */
  media?: ReactNode;
  /** The title's id, for a section labelled by it. */
  id?: string;
  className?: string;
  children?: ReactNode;
}

/** PageHeader draws the title card; children follow the subtitle inside it. */
export default function PageHeader({ title, icon, subtitle, back, above, media, id, className, children }: Props) {
  const text = (
    <>
      {back !== undefined && (
        <div className="header-back">
          <BackLink href={back.href}>{back.label}</BackLink>
        </div>
      )}
      {above}
      <h1 id={id} className="page-header-title">
        {icon !== undefined && <span aria-hidden="true">{icon}</span>}
        {title}
      </h1>
      {subtitle !== undefined && <div className="page-header-subtitle">{subtitle}</div>}
      {children}
    </>
  );
  return (
    <div className={`page-header${media === undefined ? '' : ' page-header-media'}${className === undefined ? '' : ` ${className}`}`}>
      {media === undefined ? (
        text
      ) : (
        <>
          {media}
          <div className="page-header-text">{text}</div>
        </>
      )}
    </div>
  );
}
