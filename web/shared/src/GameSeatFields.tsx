/** GameSeatFields are one player's fields in a game's result form, for organisers and players alike: how their game ended, their culture (none for one who resigned or timed out), and their turn. The form's row goes around them. */
import { turnMark } from './blitz';
import { chooseTurn, TURNS, type GameForm, type GameSide } from './game-form';

const OUTCOMES: { value: GameForm['a_outcome']; label: string }[] = [
  { value: 'finished', label: 'finished' },
  { value: 'resigned', label: 'resigned' },
  { value: 'timed_out', label: 'timed out' },
];

interface Props {
  form: GameForm;
  who: GameSide;
  name: string;
  onChange: (form: GameForm) => void;
}

export default function GameSeatFields({ form, who, name, onChange }: Props) {
  const finished = form[`${who}_outcome`] === 'finished';
  return (
    <>
      <label>
        {name}
        <select className="input-antique" value={form[`${who}_outcome`]} onChange={(e) => onChange({ ...form, [`${who}_outcome`]: e.target.value as GameForm['a_outcome'] })}>
          {OUTCOMES.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      <label>
        {name}&apos;s culture
        <input
          className="input-antique"
          type="number"
          min={0}
          step="any"
          value={form[`${who}_culture`]}
          placeholder={finished ? undefined : '0'}
          disabled={!finished}
          onChange={(e) => onChange({ ...form, [`${who}_culture`]: e.target.value })}
        />
      </label>
      <label>
        {name}&apos;s turn
        <select className="input-antique" value={form[`${who}_turn`] ?? ''} onChange={(e) => onChange(chooseTurn(form, who, Number(e.target.value)))}>
          {form[`${who}_turn`] === null && (
            <option value="" disabled>
              –
            </option>
          )}
          {TURNS.map((t) => (
            <option key={t} value={t}>
              {turnMark(t)}
              {t === 1 ? ' (went first)' : ''}
            </option>
          ))}
        </select>
      </label>
    </>
  );
}
