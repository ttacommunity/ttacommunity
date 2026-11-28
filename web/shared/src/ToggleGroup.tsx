/** A row of buttons each turning one value on or off, as Segmented's choice look; an optional first button turns them all on or off. */
import type { Option } from './Segmented';
import { allButton, toggledSet } from './toggle';

interface Props<T> {
  label: string;
  options: Option<T>[];
  chosen: ReadonlySet<T>;
  /** The label of a first button choosing every value, or none when all are chosen (with noneMeansAll, it clears the choice). */
  all?: string;
  /** Choosing none shows everything: the first button is pressed while none is chosen, and an option pressed then is chosen alone. */
  noneMeansAll?: boolean;
  onChange: (chosen: Set<T>) => void;
}

/** ToggleGroup shows each option as a button, pressed while its value is chosen. */
export default function ToggleGroup<T extends string | number>({ label, options, chosen, all, noneMeansAll = false, onChange }: Props<T>) {
  const everyOne = allButton(chosen, options.map((o) => o.value), noneMeansAll);
  return (
    <div role="group" aria-label={label} className="format-bar-group">
      {all !== undefined && (
        <button type="button" className={everyOne.pressed ? 'format-btn active' : 'format-btn'} aria-pressed={everyOne.pressed} onClick={() => onChange(everyOne.next)}>
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
