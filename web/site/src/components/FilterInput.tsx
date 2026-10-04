/** A box that narrows a table as you type, the same on every page. */

interface Props {
  value: string;
  onChange: (value: string) => void;
  /** What it filters, for screen readers, such as "Filter rivals by name". */
  label: string;
  placeholder: string;
  /** The page's own width for the box. */
  className?: string;
}

/** FilterInput reports each change as the visitor types. */
export default function FilterInput({ value, onChange, label, placeholder, className }: Props) {
  return (
    <input
      type="text"
      className={className === undefined ? 'input-antique filter-input' : `input-antique filter-input ${className}`}
      value={value}
      aria-label={label}
      placeholder={placeholder}
      enterKeyHint="search"
      autoComplete="off"
      onChange={(e) => onChange(e.target.value)}
    />
  );
}
