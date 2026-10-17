/** A tournament's format and regulations, as the site shows them. */
import { isRemoval, regulationsSavable, regulationsSource, regulationsStart } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import { refusalOf, type Outcome } from '../lib/outcome';
import Notice from './Notice';

// What the card says of the regulations the site shows now.
const REGULATIONS_SHOWN = {
  ours: "Written here, and shown on the site instead of CGE's description.",
  cge: "The site shows the latest edition's description on CGE, below. Edit it and save to show your own.",
  none: 'Neither written here nor on CGE: the site shows no box. Write it below.',
};

/** RegulationsCard edits the format and regulations the site shows, starting from CGE's text when none is written. */
export default function RegulationsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [text, setText] = useState(regulationsStart(t.regulations, t.cge_regulations));
  const [status, setStatus] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const source = regulationsSource(t.regulations, t.cge_regulations);
  const save = async (value: string) => {
    setStatus('Saving…');
    setRefusal(null);
    const r = await req.send(req.setRegulations(t.slug, value));
    setRefusal(refusalOf(r));
    if (!r.ok) {
      setStatus('');
      return;
    }
    setStatus(isRemoval(value) ? "Removed. The site shows CGE's text again." : 'Saved. The site shows it at once.');
    if (isRemoval(value)) setText(t.cge_regulations);
    await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="regulations-heading">
      <h2 id="regulations-heading" className="rating-card-heading">
        Format &amp; Regulations
      </h2>
      <p className="muted">
        {REGULATIONS_SHOWN[source]}
      </p>
      <textarea
        id="regulations-text"
        className="regulations-text"
        aria-labelledby="regulations-heading"
        value={text}
        maxLength={10000}
        rows={12}
        onChange={(e) => setText(e.target.value)}
      />
      <div className="note-actions">
        <button
          type="button"
          className="btn-antique"
          onClick={() => void save(text)}
          disabled={!regulationsSavable(text, t.regulations, t.cge_regulations)}
        >
          Save
        </button>
        {source === 'ours' && (
          <button type="button" className="btn-antique" onClick={() => void save('')}>
            {t.cge_regulations !== '' ? "Use CGE's text again" : 'Remove'}
          </button>
        )}
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
    </section>
  );
}
