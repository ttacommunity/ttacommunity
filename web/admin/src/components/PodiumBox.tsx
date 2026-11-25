/** A season's podium, set by hand or removed. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import { podiumNoteStart, podiumRequest, podiumStart, sharePlace, type PodiumEntry } from '../lib/podium';
import { fieldId } from '../lib/tournament-page';
import { ordinal } from '@ttac/shared/numbers';
import SaveBar from './SaveBar';
import { useSave } from './useSave';
import PeoplePick from './PeoplePick';
import { playerChoices } from '../lib/person-form';
import FieldGroup from './FieldGroup';

const PLACES = [1, 2, 3].map(ordinal);

// What the box's legend says of where its names came from.
const ORIGIN = {
  admin: '(set by hand)',
  recorded: '(as recorded by the organisers: set it to keep it here)',
  final_stage: "(suggested from the final stage's games: check it, then set it)",
  none: '(nothing recorded or suggested)',
};

/** PodiumBox sets a season's podium by hand, over the recorded or derived one, or removes it. */
export default function PodiumBox({
  slug,
  season,
  saved,
  savedNote,
  suggested,
  suggestedFrom,
  onReload,
}: {
  slug: string;
  season: string;
  saved: PodiumEntry[];
  savedNote: string;
  suggested: PodiumEntry[];
  suggestedFrom: string;
  onReload: () => Promise<void>;
}) {
  const start = podiumStart(saved, suggested, suggestedFrom);
  const [names, setNames] = useState(start.names);
  const [note, setNote] = useState(podiumNoteStart(savedNote, start.origin));
  const saving = useSave();
  const id = fieldId('podium', slug, season);
  const save = async () => {
    const body = podiumRequest(names, note);
    if (body === null) return;
    const r = await saving.run(() => req.send(req.setPodium(slug, season, body)), { done: 'Saved. Titles follow at the next run.' });
    if (r?.ok) await onReload();
  };
  const clear = async () => {
    const r = await saving.run(() => req.send(req.clearPodium(slug, season)), { done: 'Removed. The recorded or derived podium shows again.', working: 'Removing…' });
    if (r?.ok) await onReload();
  };
  return (
    <FieldGroup
      title={
        <>
          Season {season} podium {ORIGIN[start.origin]}
        </>
      }
    >
      <div className="podium-places">
        {PLACES.map((label, i) => (
          <div key={label} className="podium-place">
            {names[i].map((name, k) => (
              <PeoplePick key={k} label={k === 0 ? label : `${label}, shared`} value={name} choices={playerChoices} placeholder=""
                onChange={(text) => setNames(names.map((ns, j) => (j === i ? ns.map((n, m) => (m === k ? text : n)) : ns)))}
              />
            ))}
            <button type="button" className="btn-quiet btn-compact" onClick={() => setNames(sharePlace(names, i))}>
              Shared {label}
            </button>
          </div>
        ))}
        <label className="podium-note">
          Why, or where it comes from
          <input className="input-antique" id={`${id}-note`} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
        </label>
      </div>
      <SaveBar save={saving}>
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={podiumRequest(names, note) === null}>
          Set podium
        </button>
        {saved.length > 0 && (
          <button type="button" className="btn-antique" onClick={() => void clear()}>
            Use the results again
          </button>
        )}
      </SaveBar>
    </FieldGroup>
  );
}
