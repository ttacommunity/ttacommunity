/** A list's "Show" filter: the shared toggle group, each button showing or hiding one kind of row, with what it filters named for screen readers. */
import ToggleGroup from '@ttac/shared/ToggleGroup';
import type { Option } from '@ttac/shared/Segmented';

/** ShowFilter shows the kinds in options as buttons, pressed while chosen; what names the list for screen readers ("members"). */
export default function ShowFilter<T extends string>({ what, options, chosen, onChange }: { what: string; options: Option<T>[]; chosen: ReadonlySet<T>; onChange: (chosen: Set<T>) => void }) {
  return (
    <div className="show-filter">
      <span className="show-filter-label" aria-hidden="true">
        Show
      </span>
      <ToggleGroup label={`Show ${what}`} options={options} chosen={chosen} onChange={onChange} />
    </div>
  );
}
