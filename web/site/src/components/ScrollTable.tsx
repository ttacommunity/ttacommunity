/** A table that scrolls sideways on a phone, with upstream's hint saying so above it. */
import type { ReactNode } from 'react';

/** ScrollTable shows "Swipe table horizontally for {hint} ➔" (phones only, by CSS) above its table. */
export default function ScrollTable({ hint, children }: { hint: string; children: ReactNode }) {
  return (
    <>
      <div className="scroll-hint">
        <span>Swipe table horizontally for {hint} ➔</span>
      </div>
      <div className="table-responsive">{children}</div>
    </>
  );
}
