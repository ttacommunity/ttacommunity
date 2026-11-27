/** AdminCard is one card of an admin page: its heading (or, for a card of tabs, only a name for screen readers), then what it holds. Inside a card's tabs (InTabs), it is a section of that card instead, its heading a level down. */
import { createContext, useContext, type ReactNode } from 'react';

const InTab = createContext(false);

/** InTabs holds a card's tab panels: each AdminCard inside is drawn as a section of the card holding the tabs. */
export function InTabs({ children }: { children: ReactNode }) {
  return <InTab.Provider value={true}>{children}</InTab.Provider>;
}

type Heading =
  /** A card with a heading; actions sit on the heading's right, such as a button the whole card answers to. */
  | { id: string; title: ReactNode; actions?: ReactNode; label?: never }
  /** A card without a visible heading, such as one holding tabs, named for screen readers. */
  | { label: string; id?: never; title?: never; actions?: never };

type Props = Heading & {
  children: ReactNode;
};

export default function AdminCard({ id, title, actions, label, children }: Props) {
  const inTab = useContext(InTab);
  const cls = inTab ? 'admin-subsection admin-tab-section' : 'rating-card admin-card';
  if (label !== undefined) {
    return (
      <section className={cls} aria-label={label}>
        {children}
      </section>
    );
  }
  const heading = inTab ? (
    <h3 id={id}>{title}</h3>
  ) : (
    <h2 id={id} className="rating-card-heading">
      {title}
    </h2>
  );
  return (
    <section className={cls} aria-labelledby={id}>
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
