/** A highlight: one fact singled out, such as a peak season, an honour, a champion or a story. */
import type { ReactNode } from 'react';
import './Highlight.css';

/** Highlight draws the box with its gold stripe; the page lays out its own content inside. */
export default function Highlight({ children }: { children: ReactNode }) {
  return <div className="highlight">{children}</div>;
}
