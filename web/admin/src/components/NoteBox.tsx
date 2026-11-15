/** A season's or stage's note, edited in place. */
import { isRemoval, textChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import { fieldId } from '../lib/tournament-page';
import SaveBar from './SaveBar';
import { useSave } from './useSave';

/** NoteBox edits the note shown beside a season (stage '') or one of its stages. */
export default function NoteBox({
  slug,
  season,
  stage,
  initial,
  original = null,
  onSaved,
}: {
  slug: string;
  season: string;
  stage: string;
  initial: string;
  original?: string | null;
  onSaved?: () => void;
}) {
  const [text, setText] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const saving = useSave();
  const id = fieldId('note', slug, season, stage);
  const save = async () => {
    const r = await saving.run(() => req.send(req.setNote(slug, season, stage, text)), { done: isRemoval(text) ? 'Removed' : 'Saved' });
    if (r?.ok) {
      setSaved(text.trim());
      onSaved?.();
    }
  };
  return (
    <div className="note-box">
      <label htmlFor={id}>
        {stage === '' ? 'Season note' : `${stage} note`}
        {original !== null && <span className="muted"> · imported as {original}</span>}
      </label>
      <textarea className="input-antique" id={id} value={text} maxLength={2000} rows={2} onChange={(e) => setText(e.target.value)} />
      <SaveBar save={saving}>
        <button type="button" className="btn-antique" onClick={save} disabled={!textChanged(saved, text)}>
          Save note
        </button>
      </SaveBar>
    </div>
  );
}
