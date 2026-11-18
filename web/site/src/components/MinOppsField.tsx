/** A minimum-opponents box with its Apply button: the number goes through only when applied. */
import { useEffect, useState } from 'react';
import './MinOppsField.css';

interface Props {
  id: string;
  value: number;
  onApply: (minOpps: number) => void;
}

/** MinOppsField applies a whole number from 0 to 1000 and ignores anything else. */
export default function MinOppsField({ id, value, onApply }: Props) {
  const [typed, setTyped] = useState(String(value));
  useEffect(() => setTyped(String(value)), [value]);
  return (
    <form
      className="options-min-row"
      onSubmit={(e) => {
        e.preventDefault();
        const n = Number(typed);
        if (Number.isInteger(n) && n >= 0 && n <= 1000) onApply(n);
      }}
    >
      <input
        id={id}
        type="number"
        min={0}
        max={1000}
        className="input-antique"
        placeholder="Min opponents"
        value={typed}
        onChange={(e) => setTyped(e.target.value)}
      />
      <button type="submit" className="btn-antique">
        Apply
      </button>
    </form>
  );
}
