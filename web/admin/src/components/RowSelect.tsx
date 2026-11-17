/** RowSelect is a select in a table's row for one setting of that row's item: it shows the setting as it is, and a choice saves at once through onChange. */
interface Props<T extends string> {
  /** Names the select for screen readers, such as "Who decides whether “A video” is shown". */
  label: string;
  value: T;
  options: readonly { value: T; label: string }[];
  onChange: (value: T) => void;
  disabled?: boolean;
  /** Says why it is disabled, or what it chooses. */
  title?: string;
}

export default function RowSelect<T extends string>({ label, value, options, onChange, disabled, title }: Props<T>) {
  return (
    <select className="input-antique" aria-label={label} title={title} value={value} disabled={disabled} onChange={(e) => onChange(e.target.value as T)}>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}
