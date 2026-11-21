/** Figure tiles: each what it counts, the figure and a note under it, in a row. */
import type { ReactNode } from 'react';
import './MetricCard.css';

/** The figure's colour: win green, muted, gold, sky blue, a medal's silver or bronze, or the plain text colour without one. */
export type MetricTone = 'win' | 'muted' | 'gold' | 'sky' | 'silver' | 'bronze';

/** MetricGrid lays its MetricCards out in a row, two to a line on a phone; compact is the small form for a page header's facts. */
export function MetricGrid({ compact, children }: { compact?: boolean; children: ReactNode }) {
  return <div className={compact ? 'metric-grid metric-grid-compact' : 'metric-grid'}>{children}</div>;
}

type Props = {
  label: ReactNode;
  value: ReactNode;
  note?: ReactNode;
  tone?: MetricTone;
  /** A picture above the label, such as a medal. */
  icon?: string;
  /** The value is words, not a number: a smaller size, so it does not break over lines. */
  words?: boolean;
  /** A longer explanation, shown under the mouse. */
  hint?: string;
};

/** MetricCard shows one figure with what it counts above and a note below. */
export default function MetricCard({ label, value, note, tone, icon, words, hint }: Props) {
  const figure = ['metric-number', words && 'metric-words', tone !== undefined && `metric-${tone}`].filter(Boolean).join(' ');
  return (
    <div className="metric-card" title={hint}>
      {icon !== undefined && (
        <div className="metric-icon" aria-hidden="true">
          {icon}
        </div>
      )}
      <div className="metric-label">{label}</div>
      <div className={figure}>{value}</div>
      {note !== undefined && <div className="metric-note">{note}</div>}
    </div>
  );
}
