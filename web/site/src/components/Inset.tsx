/** A box inside a panel, or on the page where text needs one: a group of content such as a worked example, a row of figures, a short line such as an empty list, or one item of a list with its figures or actions. */
import type { ReactNode } from 'react';
import { insetClass, type InsetForm } from '../lib/inset';
import './Inset.css';

interface Props {
  /** block (the default) holds any content; figures lays its children out in a row; line is one short muted line; row puts its first child on the left and the rest on the right. */
  form?: InsetForm;
  role?: 'status';
  children: ReactNode;
}

/** Inset draws the box in the form asked for. */
export default function Inset({ form = 'block', role, children }: Props) {
  return (
    <div className={insetClass(form)} role={role}>
      {children}
    </div>
  );
}
