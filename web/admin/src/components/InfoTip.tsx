/** A small "i" button beside a control that shows a short note on when to use it, for touch screens as well as a mouse. */
import { useId, useState, type ReactNode } from 'react';

/** InfoTip opens its note on a tap or Enter, and closes it on another, on Escape, or when focus leaves. */
export default function InfoTip({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <span className="info-tip">
      <button
        type="button"
        className="info-tip-button"
        aria-label={label}
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen(!open)}
        onBlur={() => setOpen(false)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') setOpen(false);
        }}
      >
        i
      </button>
      <span id={id} role="note" className="info-tip-note" hidden={!open}>
        {children}
      </span>
    </span>
  );
}
