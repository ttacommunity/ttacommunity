/** A season's organiser, edited in place, with CGE's shown while none is set. */
import { isRemoval, shownSource, textChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import SaveBar from './SaveBar';
import { useSave } from './useSave';
import PeoplePick from './PeoplePick';

/** OrganiserBox edits who organised a season; cge is its CGE tournament's organiser, shown while none is set. */
export default function OrganiserBox({ slug, season, initial, cge }: { slug: string; season: string; initial: string; cge: string }) {
  const [text, setText] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const saving = useSave();
  const hint = {
    ours: cge === '' ? 'Set here. Empty it to show none.' : `Set here. Empty it to show CGE's, ${cge}, again.`,
    cge: `The site shows CGE's organiser, ${cge}, until one is set here.`,
    none: 'None shown on the site.',
  }[shownSource(saved, cge)];
  const save = async () => {
    const r = await saving.run(() => req.send(req.setOrganiser(slug, season, text)), { done: isRemoval(text) ? 'Removed' : 'Saved' });
    if (r?.ok) setSaved(text.trim());
  };
  return (
    <div className="note-box">
      <PeoplePick label="Organiser" value={text} onChange={setText} placeholder={cge} />
      <span className="muted">{hint}</span>
      <SaveBar save={saving}>
        <button type="button" className="btn-antique" onClick={save} disabled={!textChanged(saved, text)}>
          Save organiser
        </button>
      </SaveBar>
    </div>
  );
}
