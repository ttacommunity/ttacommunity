/** A season's or stage's note, edited in place. */
import { isRemoval, textChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import { fieldId } from '../lib/tournament-page';
import { refusalOf, type Outcome } from '../lib/outcome';
import Notice from './Notice';

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
  const [status, setStatus] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const id = fieldId('note', slug, season, stage);
  const save = async () => {
    setStatus('Saving…');
    setRefusal(null);
    const r = await req.send(req.setNote(slug, season, stage, text));
    setRefusal(refusalOf(r));
    if (r.ok) {
      setSaved(text.trim());
      setStatus(isRemoval(text) ? 'Removed' : 'Saved');
      onSaved?.();
    } else {
      setStatus('');
    }
  };
  return (
    <div className="note-box">
      <label htmlFor={id}>
        {stage === '' ? 'Season note' : `${stage} note`}
        {original !== null && <span className="muted"> · imported as {original}</span>}
      </label>
      <textarea className="input-antique" id={id} value={text} maxLength={2000} rows={2} onChange={(e) => setText(e.target.value)} />
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={save} disabled={!textChanged(saved, text)}>
          Save note
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
    </div>
  );
}
