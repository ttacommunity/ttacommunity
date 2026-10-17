/** A numbered season's time-out penalty, edited in place. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import { fieldId, penaltySavable, penaltyValue } from '../lib/tournament-page';
import { refusalOf, type Outcome } from '../lib/outcome';
import Notice from './Notice';

/** PenaltyBox edits the points a numbered season's standings take off for each game a player times out of. */
export default function PenaltyBox({ slug, season, initial }: { slug: string; season: string; initial: number }) {
  const [text, setText] = useState(String(initial));
  const [saved, setSaved] = useState(initial);
  const [status, setStatus] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const id = fieldId('penalty', slug, season);
  const value = penaltyValue(text);
  const save = async () => {
    if (value === null) return;
    setStatus('Saving…');
    setRefusal(null);
    const r = await req.send(req.setPenalty(slug, season, value));
    setStatus(r.ok ? 'Saved' : '');
    setRefusal(refusalOf(r));
    if (r.ok) setSaved(value);
  };
  return (
    <div className="setting">
      <label htmlFor={id}>
        Time-out penalty <span className="muted">(points per game timed out of, 0 or below, from the next engine run)</span>
      </label>{' '}
      <input className="input-antique" id={id} value={text} maxLength={8} inputMode="decimal" onChange={(e) => setText(e.target.value)} />{' '}
      <button type="button" className="btn-antique" onClick={save} disabled={!penaltySavable(value, saved)}>
        Save
      </button>{' '}
      <span className="note-status" role="status">
        {value === null ? 'A number, 0 or below' : status}
      </span>
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
    </div>
  );
}
