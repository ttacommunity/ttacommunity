/** A group of related fields on an admin page (a season's podium, a story, an outcome): the shared box inside a panel, titled by its legend. */
import type { ReactNode } from 'react';
import Inset from '@ttac/shared/Inset';

/** FieldGroup draws its fields in a box, with title as the group's legend. */
export default function FieldGroup({ title, children }: { title: ReactNode; children: ReactNode }) {
  return (
    <div className="field-group">
      <Inset>
        <fieldset className="field-group-fields">
          <legend className="field-group-title">{title}</legend>
          {children}
        </fieldset>
      </Inset>
    </div>
  );
}
