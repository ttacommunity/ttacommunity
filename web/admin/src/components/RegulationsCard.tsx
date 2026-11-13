/** A tournament's format and regulations, as the site shows them. */
import { isRemoval, regulationsSavable, shownSource, regulationsStart } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import SaveBar from './SaveBar';
import { useSave } from './useSave';

// What the card says of the regulations the site shows now.
const REGULATIONS_SHOWN = {
  ours: "Written here, and shown on the site instead of CGE's description.",
  cge: "The site shows the latest edition's description on CGE, below. Edit it and save to show your own.",
  none: 'Neither written here nor on CGE: the site shows no box. Write it below.',
};

/** RegulationsCard edits the format and regulations the site shows, starting from CGE's text when none is written. */
export default function RegulationsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [text, setText] = useState(regulationsStart(t.regulations, t.cge_regulations));
  const saving = useSave();
  const source = shownSource(t.regulations, t.cge_regulations);
  const save = async (value: string) => {
    const done = isRemoval(value) ? "Removed. The site shows CGE's text again." : 'Saved. The site shows it at once.';
    const r = await saving.run(() => req.send(req.setRegulations(t.slug, value)), { done });
    if (!r?.ok) return;
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
      <SaveBar save={saving}>
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
      </SaveBar>
    </section>
  );
}
