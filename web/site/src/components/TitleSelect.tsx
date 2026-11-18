/** A select of the titles players hold, with "All Titles" for no title filter. */
import type { TitleCode } from '@ttac/shared/ratings';

const titleLabels: Partial<Record<TitleCode, string>> = { GM: 'Grandmaster (GM)', M: 'Master (M)', P: 'Platinum (P)', G: 'Gold (G)' };

interface Props {
  id: string;
  titles: readonly TitleCode[];
  value: TitleCode | null;
  onChange: (title: TitleCode | null) => void;
}

/** TitleSelect offers titles in the order given; choosing "All Titles" gives null. */
export default function TitleSelect({ id, titles, value, onChange }: Props) {
  return (
    <select
      id={id}
      className="input-antique"
      value={value ?? 'ALL'}
      onChange={(e) => onChange(e.target.value === 'ALL' ? null : (e.target.value as TitleCode))}
    >
      <option value="ALL">All Titles</option>
      {titles.map((t) => (
        <option key={t} value={t}>
          {titleLabels[t] ?? t}
        </option>
      ))}
    </select>
  );
}
