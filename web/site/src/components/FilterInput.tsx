/** A box that narrows a table as you type, the same on every page: the player search's field and icon. */
import './SearchBox.css';

interface Props {
  value: string;
  onChange: (value: string) => void;
  /** What it filters, for screen readers, such as "Filter rivals by name". */
  label: string;
  placeholder: string;
  /** The page's own class for the box's layout. */
  className?: string;
}

/** FilterInput reports each change as the visitor types. */
export default function FilterInput({ value, onChange, label, placeholder, className }: Props) {
  return (
    <div className={className === undefined ? 'search-input-wrapper' : `search-input-wrapper ${className}`}>
      <input
        type="text"
        className="input-antique filter-input"
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
