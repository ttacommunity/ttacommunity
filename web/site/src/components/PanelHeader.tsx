/** A panel's title row: its heading, with an optional icon, and on the right what the panel says about itself (a count tag, a picker). */
import type { ReactNode } from 'react';

interface Props {
  title: ReactNode;
  icon?: string;
  id?: string;
  children?: ReactNode;
}

export default function PanelHeader({ title, icon, id, children }: Props) {
  return (
    <div className="panel-header">
      <h2 id={id} className="panel-header-title">
        {icon !== undefined && <span aria-hidden="true">{icon}</span>}
        {title}
      </h2>
      {children}
    </div>
  );
}
