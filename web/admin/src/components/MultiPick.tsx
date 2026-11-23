/** A dropdown of checkboxes for a filter that takes several choices: closed, it says what is ticked (all when nothing is). */
import { useEffect, useRef } from 'react';
import { pickSummary } from '../lib/multi-pick';

interface Props<T extends string> {
  label: string;
  options: readonly { value: T; label: string }[];
  ticked: readonly T[];
  onChange: (ticked: T[]) => void;
  /** Shown when nothing is ticked, such as "All creators". */
  all: string;
  /** Counts several ticked, such as "creators". */
  noun: string;
}

/** MultiPick is a labelled dropdown of options to tick; it closes on Escape or a click outside it. */
export default function MultiPick<T extends string>({ label, options, ticked, onChange, all, noun }: Props<T>) {
  const ref = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const close = (e: Event) => {
      const el = ref.current;
      if (el === null || !el.open) return;
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !el.contains(e.target as Node)) el.open = false;
    };
    document.addEventListener('click', close);
    document.addEventListener('keydown', close);
    return () => {
      document.removeEventListener('click', close);
      document.removeEventListener('keydown', close);
    };
  }, []);
  const toggle = (value: T) => onChange(ticked.includes(value) ? ticked.filter((t) => t !== value) : [...ticked, value]);
  return (
    <div className="multi-pick-field">
      <span className="multi-pick-label">{label}</span>
      <details className="multi-pick" ref={ref}>
        <summary className="input-antique">{pickSummary(options, ticked, all, noun)}</summary>
        <fieldset className="suggestions multi-pick-options">
          <legend className="visually-hidden">{label}</legend>
          {options.map((o) => (
            <label key={o.value}>
              <input type="checkbox" checked={ticked.includes(o.value)} onChange={() => toggle(o.value)} /> {o.label}
            </label>
          ))}
          {ticked.length > 0 && (
            <button type="button" className="btn-antique" onClick={() => onChange([])}>
              {all}
            </button>
          )}
        </fieldset>
      </details>
    </div>
  );
}
