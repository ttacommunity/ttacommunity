/** The fields of a community leaderboard edition, shared by its edit and new-edition forms. */
import { editionNumber, type EditionFieldValues, type NumberField } from '../lib/community-editions';

const TEXT: { key: Exclude<keyof EditionFieldValues, NumberField>; label: string; wide?: boolean; required?: boolean }[] = [
  { key: 'name', label: 'Name', required: true },
  { key: 'short_name', label: 'Short Name' },
  { key: 'title', label: 'Title', wide: true },
  { key: 'badge', label: 'Badge' },
  { key: 'maintainer', label: 'Maintainer' },
  { key: 'maintainer_note', label: 'Maintainer Note', wide: true },
];
const NUMBERS: { key: NumberField; label: string }[] = [
  { key: 'year', label: 'Year' },
  { key: 'counting_events', label: 'Counting Events' },
  { key: 'sort_order', label: 'Sort Order' },
];

/** EditionFields edits value's fields inside a form, calling onChange with the whole value after each change. */
export default function EditionFields<T extends EditionFieldValues>({ value, onChange }: { value: T; onChange: (next: T) => void }) {
  return (
    <>
      {TEXT.map((f) => (
        <label key={f.key} className={f.wide ? 'title-reason' : undefined}>
          {f.label}
          <input className="input-antique" value={value[f.key]} onChange={(e) => onChange({ ...value, [f.key]: e.target.value })} required={f.required} />
        </label>
      ))}
      {NUMBERS.map((f) => (
        <label key={f.key}>
          {f.label}
          <input type="number" className="input-antique" value={value[f.key]} onChange={(e) => onChange({ ...value, [f.key]: editionNumber(f.key, e.target.value) })} />
        </label>
      ))}
    </>
  );
}
