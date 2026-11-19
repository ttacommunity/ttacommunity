/** The player and matrix pages' selector block: their reset mode and format bars, then any of the page's own. */
import type { ReactNode } from 'react';
import type { Format } from '@ttac/shared/ratings';
import Segmented from '@ttac/shared/Segmented';
import * as show from '../lib/leaderboard-display';
import type { ResetMode, Track } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';
import ViewBar, { FormatPick } from './ViewBar';
import { TRACKS } from '@ttac/shared/tracks';

/**
 * ViewSelectors links each reset mode and format to the address linkReset or linkFormat gives.
 * children are the page's own bars, drawn after these.
 */
export default function ViewSelectors({
  reset,
  format,
  ranking,
  linkReset,
  linkFormat,
  linkRanking,
  children,
}: {
  reset: ResetMode;
  format: Format;
  ranking?: Track;
  linkReset: (reset: ResetMode) => Link;
  linkFormat: (format: Format) => Link;
  linkRanking?: (ranking: Track) => Link;
  children?: ReactNode;
}) {
  return (
    <div className="view-selectors">
      <ViewBar>
        <FormatPick value={format} link={linkFormat} what="rating and history" />
        <Segmented
          label="Reset mode"
          groupClass="view-group view-group-reset"
          prefix={<span className="view-bar-label">Reset Mode:</span>}
          options={show.RESET_MODES.map((m) => ({ value: m.value, label: m.name, tooltip: m.desc }))}
          value={reset}
          link={linkReset}
        />
      </ViewBar>
      {ranking !== undefined && linkRanking !== undefined && (
        <ViewBar>
          <Segmented
            label="Rating track"
            groupClass="view-group"
            prefix={<span className="view-bar-label">Track:</span>}
            options={TRACKS.map((t) => ({ value: t.value, label: t.name, tooltip: t.desc }))}
            value={ranking}
            link={linkRanking}
          />
        </ViewBar>
      )}
      {children}
    </div>
  );
}
