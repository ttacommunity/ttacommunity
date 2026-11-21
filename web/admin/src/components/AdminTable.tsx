/** AdminTable is an admin list as a table that scrolls sideways on a narrow screen, captioned for screen readers (or for everyone, with captionShown); its children are the head and body. */
import type { ReactNode } from 'react';

interface Props {
  caption: ReactNode;
  /** Shows the caption above the table, where it is the table's only title. */
  captionShown?: boolean;
  children: ReactNode;
}

export default function AdminTable({ caption, captionShown = false, children }: Props) {
  return (
    <div className="table-responsive">
      <table className="leaderboard-table admin-table">
        <caption className={captionShown ? undefined : 'visually-hidden'}>{caption}</caption>
        {children}
      </table>
    </div>
  );
}
