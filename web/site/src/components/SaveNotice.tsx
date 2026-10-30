/** SaveNotice shows what a save did, as admin shows an action's outcome: Done or Not done on a panel bordered green or red, scrolled into view, closed by its ×. */
import { useEffect, useRef } from 'react';
import type { SaveOutcome } from '../lib/account';
import './SaveNotice.css';

export default function SaveNotice({ outcome, onClose }: { outcome: SaveOutcome | null; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (outcome !== null) box.current?.scrollIntoView({ block: 'nearest' });
  }, [outcome]);
  if (outcome === null) return null;
  return (
    <div ref={box} className={`save-outcome save-outcome-${outcome.kind}`} role="status">
      <div>
        <strong className="save-outcome-heading">{outcome.heading}</strong>
        <span className="save-outcome-text">{outcome.text}</span>
      </div>
      <button type="button" className="save-outcome-close" aria-label="Close" onClick={onClose}>
        ×
      </button>
    </div>
  );
}
