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
  /** An emoji beside the text in a frame, as a tournament's emblem; in place of media. */
  emblem?: string;
  /** A short tag at the top right, such as a tournament's tagline; under the picture on a phone. */
  tag?: ReactNode;
  /** The title's id, for a section labelled by it. */
  id?: string;
  /** Centred, the title in the heading face, as a player's pages are. */
  centred?: boolean;
  children?: ReactNode;
}

/** PageHeader draws the title card; children follow the subtitle inside it. */
export default function PageHeader({ title, icon, subtitle, back, above, media, emblem, tag, id, centred = false, children }: Props) {
  const text = (
    <>
      {tag !== undefined && <div className="tag page-header-tag">{tag}</div>}
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
  const picture =
    emblem === undefined ? (
      media
    ) : (
      <div className="page-header-emblem" aria-hidden="true">
        {emblem}
      </div>
    );
  return (
    <div className={`page-header${picture === undefined ? '' : ' page-header-media'}${centred ? ' page-header-centred' : ''}`}>
      {picture === undefined ? (
        text
      ) : (
        <>
          {picture}
          <div className="page-header-text">{text}</div>
        </>
      )}
    </div>
  );
}
