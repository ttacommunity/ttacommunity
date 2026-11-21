/** A box that narrows a list that is not of players as you type, drawn as the player search is. Player lists use PlayerSearch. */
import './PlayerSearch.css';

interface Props {
  value: string;
  onChange: (value: string) => void;
  /** What it filters, for screen readers, such as "Filter tournaments". */
  label: string;
  /** What it filters, shown in the empty box. */
  placeholder: string;
}

/** FilterInput reports each change as the visitor types. */
export default function FilterInput({ value, onChange, label, placeholder }: Props) {
  return (
    <div className="search-input-wrapper">
      <input
        type="text"
        className="input-antique"
        value={value}
        aria-label={label}
        placeholder={placeholder}
        enterKeyHint="search"
        autoComplete="off"
        onChange={(e) => onChange(e.target.value)}
      />
      <span className="btn-icon search-submit search-icon" aria-hidden="true">
        🔍
      </span>
    </div>
  );
}
