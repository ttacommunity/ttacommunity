/** A row of buttons each turning one value on or off, as Segmented's choice look; an optional first button turns them all on or off. */
import type { Option } from './Segmented';
import { toggledAll, toggledSet } from './toggle';

interface Props<T> {
  label: string;
  options: Option<T>[];
  chosen: ReadonlySet<T>;
  /** The label of a first button choosing every value, or none when all are chosen. */
  all?: string;
  onChange: (chosen: Set<T>) => void;
}

/** ToggleGroup shows each option as a button, pressed while its value is chosen. */
export default function ToggleGroup<T extends string | number>({ label, options, chosen, all, onChange }: Props<T>) {
  const every = options.map((o) => o.value);
  const allChosen = every.every((v) => chosen.has(v));
  return (
    <div role="group" aria-label={label} className="format-bar-group">
      {all !== undefined && (
        <button type="button" className={allChosen ? 'format-btn active' : 'format-btn'} aria-pressed={allChosen} onClick={() => onChange(toggledAll(chosen, every))}>
          {all}
        </button>
      )}
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          className={chosen.has(o.value) ? 'format-btn active' : 'format-btn'}
          aria-pressed={chosen.has(o.value)}
          data-tooltip={o.tooltip}
          title={o.title}
          onClick={() => onChange(toggledSet(chosen, o.value))}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
