/** A box inside a panel, or on the page where text needs one: a group of content such as a worked example, a row of figures, or a short line such as an empty list. */
import type { ReactNode } from 'react';
import './Inset.css';

interface Props {
  /** block (the default) holds any content; figures lays its children out in a row; line is one short muted line. */
  form?: 'block' | 'figures' | 'line';
  role?: 'status';
  /** The page's own class for the box's layout inside it, never its look. */
  className?: string;
  children: ReactNode;
}

/** Inset draws the box in the form asked for. */
export default function Inset({ form = 'block', role, className, children }: Props) {
  const look = form === 'block' ? 'inset' : `inset inset-${form}`;
  return (
    <div className={className === undefined ? look : `${look} ${className}`} role={role}>
      {children}
    </div>
  );
}
