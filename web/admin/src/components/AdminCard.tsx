/** AdminCard is one card of an admin page: its heading, then what it holds. */
import type { ReactNode } from 'react';

interface Props {
  id: string;
  title: ReactNode;
  className?: string;
  children: ReactNode;
}

export default function AdminCard({ id, title, className, children }: Props) {
  return (
    <section className={`rating-card admin-card${className === undefined ? '' : ` ${className}`}`} aria-labelledby={id}>
      <h2 id={id} className="rating-card-heading">
        {title}
      </h2>
      {children}
    </section>
  );
}
