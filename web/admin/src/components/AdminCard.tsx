/** AdminCard is one card of an admin page: its heading (or, for a card of tabs, only a name for screen readers), then what it holds. */
import type { ReactNode } from 'react';

type Heading =
  /** A card with a heading; actions sit on the heading's right, such as a button the whole card answers to. */
  | { id: string; title: ReactNode; actions?: ReactNode; label?: never }
  /** A card without a visible heading, such as one holding tabs, named for screen readers. */
  | { label: string; id?: never; title?: never; actions?: never };

type Props = Heading & {
  className?: string;
  /** alert, for a card that replaces the page to say what went wrong. */
  role?: 'alert';
  children: ReactNode;
};

export default function AdminCard({ id, title, actions, label, className, role, children }: Props) {
  const cls = `rating-card admin-card${className === undefined ? '' : ` ${className}`}`;
  if (label !== undefined) {
    return (
      <section className={cls} role={role} aria-label={label}>
        {children}
      </section>
    );
  }
  const heading = (
    <h2 id={id} className="rating-card-heading">
      {title}
    </h2>
  );
  return (
    <section className={cls} role={role} aria-labelledby={id}>
      {actions === undefined ? (
        heading
      ) : (
        <div className="admin-card-head">
          {heading}
          {actions}
        </div>
      )}
      {children}
    </section>
  );
}
