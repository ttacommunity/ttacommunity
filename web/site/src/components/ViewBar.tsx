/** A page's bar of view choices above its content: the format and the like on the left, the Options button at the right. */
import type { ReactNode } from 'react';
import type { Format } from '@ttac/shared/ratings';
import Segmented from '@ttac/shared/Segmented';
import { formatName } from '../lib/leaderboard-display';
import { FORMATS } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';
import './ViewBar.css';

/** ViewBar holds a page's view choices in one full-width bar; actions (a search, the Options button) keep together at its right, or on the next line when the bar is narrow. */
export default function ViewBar({ children, actions }: { children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="view-bar">
      {children}
      {actions !== undefined && <div className="view-bar-actions">{actions}</div>}
    </div>
  );
}

interface FormatProps {
  value: Format;
  /** Each format as a link, or else a button calling onChange. */
  link?: (format: Format) => Link;
  onChange?: (format: Format) => void;
  /** What choosing a format shows, for its tooltip: "View <what> for 3-Player". */
  what: string;
}

/** FormatPick is a ViewBar's "Format:" choice of all formats or one player count. */
export function FormatPick({ value, link, onChange, what }: FormatProps) {
  return (
    <Segmented
      label="Format"
      prefix={<span className="view-bar-label">Format:</span>}
      options={FORMATS.map((f) => ({ value: f, label: formatName(f), tooltip: `View ${what} for ${formatName(f)}` }))}
      value={value}
      link={link}
      onChange={onChange}
    />
  );
}
