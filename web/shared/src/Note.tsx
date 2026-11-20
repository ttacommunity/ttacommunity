/** Note is an aside a page puts in front of the reader, such as what to do before taking part: a gold-edged box with an optional title, red for a warning. */
import type { ReactNode } from 'react';

interface Props {
  /** A line above the text, on its own. */
  title?: string;
  /** Gold by default; warning for something that stops the reader, such as a ban. */
  tone?: 'gold' | 'warning';
  children: ReactNode;
}

export default function Note({ title, tone = 'gold', children }: Props) {
  return (
    <div className={tone === 'warning' ? 'page-note page-note-warning' : 'page-note'} role="note">
      {title !== undefined && <strong className="page-note-title">{title}</strong>}
      {children}
    </div>
  );
}
