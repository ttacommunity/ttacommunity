/** The button under every long table: it shows all its rows, or its first ones again. */
import { ROWS_SHOWN, cutsRows } from '../lib/show-all';
import './ShowAll.css';

/** ShowAll draws nothing for a table that is not cut; total is the table's row count. */
export default function ShowAll({ total, expanded, onToggle }: { total: number; expanded: boolean; onToggle: () => void }) {
  if (!cutsRows(total)) return null;
  return (
    <div className="show-all">
      <button type="button" className="btn-antique" aria-expanded={expanded} onClick={onToggle}>
        {expanded ? `- Show the first ${ROWS_SHOWN}` : `+ Show all ${total}`}
      </button>
    </div>
  );
}
