/** One way to say a page's data is loading, or could not load: a card for a page, a line in a panel, a row in a table. */
import { waitingText, type LoadState } from '../lib/waiting';

interface Props {
  load: LoadState;
  /** What is loading, as it reads after "Loading", such as "this player". */
  what: string;
  form?: 'page' | 'inline' | 'row';
  /** The table's column count, for a row. */
  colSpan?: number;
}

/** Waiting shows nothing once loaded. */
export default function Waiting({ load, what, form = 'page', colSpan }: Props) {
  const text = waitingText(load, what);
  if (text === null) return null;
  const role = load.kind === 'error' ? 'alert' : 'status';
  if (form === 'row') {
    return (
      <tr>
        <td colSpan={colSpan} className="waiting-row" role={role}>
          {text}
        </td>
      </tr>
    );
  }
  if (form === 'inline') {
    return (
      <p className="waiting-inline" role={role}>
        {text}
      </p>
    );
  }
  return (
    <div className="rating-card waiting-message" role={role}>
      {text}
    </div>
  );
}
