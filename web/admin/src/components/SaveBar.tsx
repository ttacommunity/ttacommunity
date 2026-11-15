/** A box's save: its buttons in a row with the save's word beside them, and the refusal under them. */
import type { ReactNode } from 'react';
import { shownAt } from '../lib/outcome';
import { statusText } from '../lib/save';
import Notice from './Notice';
import type { Save } from './useSave';

/** SaveBar puts children (the box's buttons) in a row, then save's word, with its refusal below. */
export default function SaveBar({ save, children }: { save: Save; children: ReactNode }) {
  return (
    <>
      <div className="note-actions">
        {children}
        <SaveStatus save={save} />
      </div>
      <Notice outcome={save.refusal} onClose={save.close} />
    </>
  );
}

/** SaveStatus is save's word alone, for a box whose buttons sit in a line of text; prefix comes before the word (a count, say). */
export function SaveStatus({ save, prefix = '' }: { save: Save; prefix?: string }) {
  return (
    <span className="note-status" role="status">
      {statusText(prefix, save.status)}
    </span>
  );
}

/** SaveWord is save's word for one place on a box (at), for a box whose controls each save at once; nothing elsewhere. */
export function SaveWord({ save, at }: { save: Save; at: string }) {
  if (save.at !== at || save.status === '') return null;
  return (
    <span className="note-status" role="status">
      {save.status}
    </span>
  );
}

/** PlacedRefusal is save's refusal for one place on a box (at), under it. */
export function PlacedRefusal({ save, at }: { save: Save; at: string }) {
  return <Notice outcome={shownAt(save.refusal, at)} onClose={save.close} />;
}
