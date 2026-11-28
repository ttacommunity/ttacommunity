/** The new game's fields in a game change: its replay code, day, number in its group (for an addition), and 2 to 4 seats, each a player the ratings know with their place, outcome and culture. */
import { playerChoices } from '../lib/person-form';
import { OUTCOMES, withoutSeat, withSeat, type EntryForm, type SeatForm, type SeatOutcome } from '../lib/game-changes';
import PeoplePick from './PeoplePick';

interface Props {
  entry: EntryForm;
  onChange: (entry: EntryForm) => void;
  /** next is the number the game gets when none is typed, for an addition; null for a replacement, which takes the original's. */
  next: number | null;
}

const PLACES = ['1', '2', '3', '4'];

export default function GameEntryFields({ entry, onChange, next }: Props) {
  const setSeat = (i: number, s: SeatForm) => onChange({ ...entry, seats: entry.seats.map((x, j) => (j === i ? s : x)) });
  return (
    <>
      <div className="field-row">
        <label>
          Replay code
          <input className="input-antique" value={entry.code} maxLength={64} required onChange={(e) => onChange({ ...entry, code: e.target.value })} />
        </label>
        <label>
          Day played
          <input className="input-antique" type="date" value={entry.date} required onChange={(e) => onChange({ ...entry, date: e.target.value })} />
        </label>
        {next !== null && (
          <label>
            Game number (the group&apos;s next if empty)
            <input className="input-antique" type="number" min={1} step={1} value={entry.number} placeholder={String(next)}
              onChange={(e) => onChange({ ...entry, number: e.target.value })} />
          </label>
        )}
      </div>
      {entry.seats.map((s, i) => (
        <div className="field-row" key={i}>
          <PeoplePick label={`Player ${i + 1}`} value={s.player} choices={playerChoices} placeholder="A player the ratings know"
            onChange={(player) => setSeat(i, { ...s, player })} />
          <label>
            Place
            <select className="input-antique" value={s.place} onChange={(e) => setSeat(i, { ...s, place: e.target.value })}>
              {PLACES.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </label>
          <label>
            Ended
            <select className="input-antique" value={s.outcome} onChange={(e) => setSeat(i, { ...s, outcome: e.target.value as SeatOutcome })}>
              {OUTCOMES.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            Culture
            <input className="input-antique" type="number" min={0} step="any" value={s.outcome === 'finished' ? s.culture : ''} placeholder={s.outcome === 'finished' ? undefined : '0'}
              disabled={s.outcome !== 'finished'} onChange={(e) => setSeat(i, { ...s, culture: e.target.value })} />
          </label>
          {entry.seats.length > 2 && (
            <button type="button" className="btn-quiet btn-compact" onClick={() => onChange(withoutSeat(entry, i))}>
              Take away player {i + 1}
            </button>
          )}
        </div>
      ))}
      {entry.seats.length < 4 && (
        <button type="button" className="btn-antique btn-compact" onClick={() => onChange(withSeat(entry))}>
          Add a player
        </button>
      )}
    </>
  );
}
