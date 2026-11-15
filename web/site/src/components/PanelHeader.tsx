/** A panel's title row: its heading, with an optional icon and a line under it, and on the right what the panel says about itself (a count tag, a picker). */
import type { ReactNode } from 'react';

interface Props {
  title: ReactNode;
  icon?: string;
  id?: string;
  /** A line under the title, saying what the panel shows. */
  subtitle?: ReactNode;
  children?: ReactNode;
}

export default function PanelHeader({ title, icon, id, subtitle, children }: Props) {
  const heading = (
    <h2 id={id} className="panel-header-title">
      {icon !== undefined && <span aria-hidden="true">{icon}</span>}
      {title}
    </h2>
  );
  return (
    <div className="panel-header">
      {subtitle === undefined ? (
        heading
      ) : (
        <div>
          {heading}
          <p className="panel-header-sub">{subtitle}</p>
        </div>
      )}
      {children}
    </div>
  );
}
