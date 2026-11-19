/** A row of headline figures: each card a label, a big value and a note under it. */
import type { ReactNode } from 'react';
import './MetricCard.css';

/** The value's colour: win green, muted, gold or sky blue, or the plain text colour without one. */
export type MetricTone = 'win' | 'muted' | 'gold' | 'sky';

/** MetricGrid lays its MetricCards out in a row, two to a line on a phone. */
export function MetricGrid({ children }: { children: ReactNode }) {
  return <div className="metric-grid">{children}</div>;
}

/** MetricCard shows one figure with what it counts above and a note below. */
export default function MetricCard({ label, value, note, tone }: { label: ReactNode; value: ReactNode; note?: ReactNode; tone?: MetricTone }) {
  return (
    <div className="metric-card">
      <div className="metric-label">{label}</div>
      <div className={tone === undefined ? 'metric-number' : `metric-number metric-${tone}`}>{value}</div>
      {note !== undefined && <div className="metric-note">{note}</div>}
    </div>
  );
}
