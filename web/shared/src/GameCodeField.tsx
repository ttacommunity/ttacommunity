/** A game's share or replay code, with its label: typed in any case, shown in capitals as it is stored. */
import type { KeyboardEvent } from 'react';
import { typedCode } from './game-form';

interface Props {
  /** label names the code, as the form calls it. */
  label: string;
  value: string;
  onChange: (code: string) => void;
  /** required when the form cannot be sent without a code. */
  required?: boolean;
  /** onEnter runs when Enter is pressed in the field, in place of sending its form. */
  onEnter?: () => void;
}

export default function GameCodeField({ label, value, onChange, required = false, onEnter }: Props) {
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (onEnter === undefined || e.key !== 'Enter') return;
    e.preventDefault();
    onEnter();
  };
  return (
    <label>
      {label}
      <input
        className="input-antique"
        value={value}
        maxLength={32}
        required={required}
        autoCapitalize="characters"
        spellCheck={false}
        onChange={(e) => onChange(typedCode(e.target.value))}
        onKeyDown={onKeyDown}
      />
    </label>
  );
}
