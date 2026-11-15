/** SeasonPick picks one of a tournament's seasons, newest first, for a panel's title row; with only one season it names it. */
import { seasonName } from '../lib/catalogue-view';

interface Props {
  id: string;
  seasons: readonly { label: string }[];
  value: string;
  word: string;
  onChange: (label: string) => void;
}

export default function SeasonPick({ id, seasons, value, word, onChange }: Props) {
  if (seasons.length <= 1) return <span className="panel-tag">{seasonName(value, word)}</span>;
  return (
    <label>
      <span className="visually-hidden">{word}</span>
      <select id={id} className="input-antique" value={value} onChange={(e) => onChange(e.target.value)}>
        {[...seasons].reverse().map((x) => (
          <option key={x.label} value={x.label}>
            {seasonName(x.label, word)}
          </option>
        ))}
      </select>
    </label>
  );
}
