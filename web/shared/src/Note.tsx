/** Note is a short aside a page puts in front of the reader, such as what to do before taking part: a gold-edged box. */
import type { ReactNode } from 'react';

export default function Note({ children }: { children: ReactNode }) {
  return (
    <p className="page-note" role="note">
      {children}
    </p>
  );
}
