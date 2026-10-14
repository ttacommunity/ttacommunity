/** The way back to a list or the page above, as upstream's pages have it: an antique button starting with «. */
import type { ReactNode } from 'react';

/** BackLink links back to href, its text after the «. */
export default function BackLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <a href={href} className="btn-antique">
      « {children}
    </a>
  );
}
