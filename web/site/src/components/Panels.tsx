/** A page's panels, one under another with the site's gap between them; a row of panels side by side sits in it as one item. */
import type { ReactNode } from 'react';

export default function Panels({ children }: { children: ReactNode }) {
  return <div className="panels">{children}</div>;
}
