/** The player and matrix pages' selector block: their reset mode and format bars, then any of the page's own. */
import type { ReactNode } from 'react';
import type { Format } from '@ttac/shared/ratings';
import Segmented from '@ttac/shared/Segmented';
import * as show from '../lib/leaderboard-display';
import type { ResetMode } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';

const FORMATS: Format[] = [0, 2, 3, 4];
const RESET_MODES: { value: ResetMode; name: string; desc: string }[] = [
  {
    value: 'continuous',
    name: 'Continuous (Career)',
    desc: 'Standard uninterrupted career rating trajectory across all tournament history',
  },
  { value: 'softer', name: 'Season Reset', desc: 'Gentle annual reset balancing career achievement with current season form' },
];

/**
 * ViewSelectors links each reset mode and format to the address linkReset or linkFormat gives.
 * children are the page's own bars, drawn after these.
 */
export default function ViewSelectors({
  reset,
  format,
  linkReset,
  linkFormat,
  children,
}: {
  reset: ResetMode;
  format: Format;
  linkReset: (reset: ResetMode) => Link;
  linkFormat: (format: Format) => Link;
  children?: ReactNode;
}) {
  return (
    <div className="view-selectors">
      <div className="view-bar view-bar-row">
        <Segmented
          label="Format"
          look="raised"
          groupClass="view-group view-group-format"
          prefix={<span className="view-bar-label">Format:</span>}
          options={FORMATS.map((f) => ({ value: f, label: show.formatName(f), tooltip: `View rating and history for ${show.formatName(f)}` }))}
          value={format}
          link={linkFormat}
        />
        <Segmented
          label="Reset mode"
          look="raised"
          groupClass="view-group view-group-reset"
          prefix={<span className="view-bar-label">Reset Mode:</span>}
          options={RESET_MODES.map((m) => ({ value: m.value, label: m.name, tooltip: m.desc }))}
          value={reset}
          link={linkReset}
        />
      </div>
      {children}
    </div>
  );
}
