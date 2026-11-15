/** AdminTable is an admin list as a table that scrolls sideways on a narrow screen, captioned for screen readers; its children are the head and body. */
import type { ReactNode } from 'react';

interface Props {
  caption: ReactNode;
  className?: string;
  children: ReactNode;
}

export default function AdminTable({ caption, className, children }: Props) {
  return (
    <div className="table-responsive">
      <table className={`leaderboard-table admin-table${className === undefined ? '' : ` ${className}`}`}>
        <caption className="visually-hidden">{caption}</caption>
        {children}
      </table>
    </div>
  );
}
