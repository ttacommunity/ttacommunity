/** An action's outcome beside the action: its word and text on a panel bordered green or red, scrolled into view and read out by the page's live regions. */
import { useEffect, useMemo, useRef } from 'react';
import { announcement, loadFailure, noticeHeading, revealed, type Outcome } from '../lib/outcome';

// The live regions AdminLayout puts on every page, empty until an outcome is announced.
const REGIONS = { alert: 'notice-alert', status: 'notice-status' } as const;

/** announce has a screen reader read an outcome from the region it belongs in, and empties the other; null empties both. */
function announce(o: Outcome | null) {
  const a = announcement(o);
  for (const [region, id] of Object.entries(REGIONS)) {
    const el = document.getElementById(id);
    if (a !== null && a.region === region) el?.replaceChildren(a.text);
    else el?.replaceChildren();
  }
}

/** Notice shows an outcome, or nothing for null, with a close button when onClose is given. Spans throughout, so it fits inside a paragraph too. */
export default function Notice({ outcome, onClose }: { outcome: Outcome | null; onClose?: () => void }) {
  const box = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (outcome === null) return;
    announce(outcome);
    if (revealed(outcome)) box.current?.scrollIntoView({ block: 'nearest' });
    return () => announce(null);
  }, [outcome]);
  if (outcome === null) return null;
  return (
    <span ref={box} className={`notice notice-${outcome.kind}`}>
      <span className="notice-body">
        <strong className="notice-heading">{noticeHeading(outcome.kind)}</strong>
        <span className="notice-text">{outcome.text}</span>
      </span>
      {onClose !== undefined && (
        <button type="button" className="notice-close" aria-label="Close" onClick={onClose}>
          ×
        </button>
      )}
    </span>
  );
}

/** NoticeRow shows an outcome in a table row of its own, spanning the table's columns, under the row it is about. */
export function NoticeRow({ outcome, columns, onClose }: { outcome: Outcome | null; columns: number; onClose: () => void }) {
  if (outcome === null) return null;
  return (
    <tr className="notice-row">
      <td colSpan={columns}>
        <Notice outcome={outcome} onClose={onClose} />
      </td>
    </tr>
  );
}

/** LoadNotice shows why something could not load, in place of it. */
export function LoadNotice({ message }: { message: string }) {
  const outcome = useMemo(() => loadFailure(message, ''), [message]);
  return <Notice outcome={outcome} />;
}
