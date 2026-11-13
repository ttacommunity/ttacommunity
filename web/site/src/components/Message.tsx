/** One way to put a message where a page's content would be: a card for a page, a line in a panel, a row in a table. */
import type { ReactNode } from 'react';

interface Props {
  children: ReactNode;
  form?: 'page' | 'inline' | 'row';
  /** The table's column count, for a row. */
  colSpan?: number;
  role?: 'status' | 'alert';
}

/** Message shows children in the form asked for, with the role given (none by default). */
export default function Message({ children, form = 'page', colSpan, role }: Props) {
  if (form === 'row') {
    return (
      <tr>
        <td colSpan={colSpan} className="waiting-row" role={role}>
          {children}
        </td>
      </tr>
    );
  }
  if (form === 'inline') {
    return (
      <p className="waiting-inline" role={role}>
        {children}
      </p>
    );
  }
  return (
    <div className="rating-card waiting-message" role={role}>
      {children}
    </div>
  );
}
