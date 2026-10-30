/** A season's organiser, edited in place, with CGE's shown while none is set. */
import { isRemoval, shownSource, textChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import { fieldId } from '../lib/tournament-page';
import { refusalOf, type Outcome } from '../lib/outcome';
import Notice from './Notice';

/** OrganiserBox edits who organised a season; cge is its CGE tournament's organiser, shown while none is set. */
export default function OrganiserBox({ slug, season, initial, cge }: { slug: string; season: string; initial: string; cge: string }) {
  const [text, setText] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [status, setStatus] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const id = fieldId('organiser', slug, season);
  const hint = {
    ours: cge === '' ? 'Set here. Empty it to show none.' : `Set here. Empty it to show CGE's, ${cge}, again.`,
    cge: `The site shows CGE's organiser, ${cge}, until one is set here.`,
    none: 'None shown on the site.',
  }[shownSource(saved, cge)];
  const save = async () => {
    setStatus('Saving…');
    setRefusal(null);
    const r = await req.send(req.setOrganiser(slug, season, text));
    setRefusal(refusalOf(r));
    if (r.ok) {
      setSaved(text.trim());
      setStatus(isRemoval(text) ? 'Removed' : 'Saved');
    } else {
      setStatus('');
    }
  };
  return (
    <div className="note-box">
      <label htmlFor={id}>
        Organiser <span className="muted">· {hint}</span>
      </label>
      <input className="input-antique" id={id} value={text} maxLength={100} placeholder={cge} onChange={(e) => setText(e.target.value)} />
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={save} disabled={!textChanged(saved, text)}>
          Save organiser
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
    </div>
  );
}
