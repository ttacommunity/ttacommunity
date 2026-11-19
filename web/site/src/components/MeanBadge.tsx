/** An engine's mean rating as a small label in its chart colour, beside a distribution chart's title. */
import type { ReactNode } from 'react';
import { engineName } from '../lib/leaderboard-display';
import type { Engine } from '../lib/leaderboard-view';
import './MeanBadge.css';

/** MeanBadges lays out a chart's MeanBadges in a row. */
export function MeanBadges({ children }: { children: ReactNode }) {
  return <div className="mean-badges">{children}</div>;
}

/** MeanBadge shows engine's mean, and its active players' mean when given. */
export default function MeanBadge({ engine, mean, active }: { engine: Engine; mean: number; active?: number }) {
  return (
    <span className={`mean-badge mean-${engine}`}>
      {engineName(engine)} Mean: <strong>{mean.toFixed(1)}</strong>
      {active !== undefined && <small> (Active: {active.toFixed(1)})</small>}
    </span>
  );
}
