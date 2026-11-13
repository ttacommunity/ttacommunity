/** An action's outcome: its word and text on a dark panel edged green when done, red when not, with a close button when onClose is given. Spans throughout, so it fits inside a paragraph too. */
import { useEffect, useRef } from 'react';
import { noticeHeading, type OutcomeKind } from './outcome';

/** OutcomeNotice shows kind's word and text, scrolled into view when scroll is set. */
export default function OutcomeNotice({ kind, text, onClose, scroll = true }: { kind: OutcomeKind; text: string; onClose?: () => void; scroll?: boolean }) {
  const box = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (scroll) box.current?.scrollIntoView({ block: 'nearest' });
  }, [kind, text, scroll]);
  return (
    <span ref={box} className={`notice notice-${kind}`}>
      <span className="notice-body">
        <strong className="notice-heading">{noticeHeading(kind)}</strong>
        <span className="notice-text">{text}</span>
      </span>
      {onClose !== undefined && (
        <button type="button" className="notice-close" aria-label="Close" onClick={onClose}>
          ×
        </button>
      )}
    </span>
  );
}
