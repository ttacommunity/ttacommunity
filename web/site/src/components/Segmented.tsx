/** A row of buttons choosing one value, drawn as upstream's format bar. */

export interface Option<T> {
  value: T;
  label: string;
}

interface Props<T> {
  label: string;
  options: Option<T>[];
  value: T;
  onChange: (value: T) => void;
}

/** Segmented shows each option as a button, with the chosen one pressed. */
export default function Segmented<T extends string | number>({ label, options, value, onChange }: Props<T>) {
  return (
    <div role="group" aria-label={label} className="format-bar-group">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          className={o.value === value ? 'format-btn active' : 'format-btn'}
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
