/** A text field offering choices as you type, picked with the mouse, or the arrow keys and Enter (WAI-ARIA's combobox with a list). */
import { useId, useState, type KeyboardEvent } from 'react';
import { enterAction } from '../lib/pick';

/** PickOption is one choice: its label, a line saying more, and whether it may be picked. */
export type PickOption = { key: string; label: string; detail?: string; disabled?: boolean };

type Props = {
  label: string;
  text: string;
  options: readonly PickOption[];
  onText: (text: string) => void;
  onPick: (option: PickOption) => void;
  /** onLeave is given the text when the field loses focus. */
  onLeave?: (text: string) => void;
  placeholder?: string;
  disabled?: boolean;
  /** hideLabel keeps the label for screen readers only, for a field in a table row. */
  hideLabel?: boolean;
};

/** PickBox shows its options while focused; Enter does what enterAction says. */
export default function PickBox({ label, text, options, onText, onPick, onLeave, placeholder, disabled = false, hideLabel = false }: Props) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const [typing, setTyping] = useState(false);
  const shown = open ? options : [];
  const chosen = active >= 0 && active < shown.length ? active : -1;
  const pick = (o: PickOption) => {
    if (o.disabled === true) return;
    setOpen(false);
    setActive(-1);
    setTyping(false);
    onPick(o);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setOpen(true);
      const n = options.length;
      const step = e.key === 'ArrowDown' ? 1 : -1;
      if (n > 0) setActive((i) => (i < 0 || i >= n ? (step > 0 ? 0 : n - 1) : (i + step + n) % n));
    } else if (e.key === 'Enter') {
      const act = enterAction(shown, chosen, open && typing);
      if (act.kind !== 'send') e.preventDefault();
      if (act.kind === 'pick') pick(shown[act.index]);
    } else if (e.key === 'Escape' && shown.length > 0) {
      e.preventDefault();
      setOpen(false);
    }
  };
  return (
    <div className="pick-field">
      <label htmlFor={id} className={hideLabel ? 'visually-hidden' : undefined}>
        {label}
      </label>
      <div className="pick-box">
        <input
          id={id}
          className="input-antique"
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={shown.length > 0}
          aria-controls={`${id}-list`}
          aria-activedescendant={chosen >= 0 ? `${id}-${chosen}` : undefined}
          autoComplete="off"
          spellCheck={false}
          placeholder={placeholder}
          disabled={disabled}
          value={text}
          onChange={(e) => {
            onText(e.target.value);
            setOpen(true);
            setActive(-1);
            setTyping(true);
          }}
          onFocus={(e) => {
            e.target.select();
            setOpen(true);
            setTyping(false);
          }}
          onBlur={() => {
            setOpen(false);
            setActive(-1);
            setTyping(false);
            onLeave?.(text);
          }}
          onKeyDown={onKeyDown}
        />
        <div id={`${id}-list`} role="listbox" aria-label={label} className="pick-options" hidden={shown.length === 0}>
          {shown.map((o, i) => (
            <div
              key={o.key}
              id={`${id}-${i}`}
              role="option"
              tabIndex={-1}
              aria-selected={i === chosen}
              aria-disabled={o.disabled === true ? true : undefined}
              className={i === chosen ? 'pick-option active' : 'pick-option'}
              onMouseEnter={() => setActive(i)}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(o);
              }}
            >
              <span className="pick-label">{o.label}</span>
              {o.detail !== undefined && o.detail !== '' && <span className="pick-detail">{o.detail}</span>}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
