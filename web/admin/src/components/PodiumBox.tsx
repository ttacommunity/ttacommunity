/** A season's podium, set by hand or removed. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import { podiumNoteStart, podiumRequest, podiumStart, type PodiumEntry } from '../lib/podium';
import { fieldId } from '../lib/tournament-page';

const PLACES = ['1st', '2nd', '3rd'];

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
  const [status, setStatus] = useState('');
  const id = fieldId('podium', slug, season);
  const save = async () => {
    const body = podiumRequest(names, note);
    if (body === null) return;
    setStatus('Saving…');
    const r = await req.send(req.setPodium(slug, season, body));
    setStatus(r.ok ? 'Saved. Titles follow at the next run.' : r.message);
    if (r.ok) await onReload();
  };
  const clear = async () => {
    setStatus('Removing…');
    const r = await req.send(req.clearPodium(slug, season));
    setStatus(r.ok ? 'Removed. The recorded or derived podium shows again.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <fieldset className="podium-box">
      <legend>
        Season {season} podium {ORIGIN[start.origin]}
      </legend>
      <div className="podium-places">
        {PLACES.map((label, i) => (
          <label key={label}>
            {label}
            <input className="input-antique"
              id={`${id}-${i + 1}`}
              value={names[i]}
              maxLength={64}
              onChange={(e) => setNames(names.map((n, j) => (j === i ? e.target.value : n)))}
            />
          </label>
        ))}
        <label className="podium-note">
          Why, or where it comes from
          <input className="input-antique" id={`${id}-note`} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
        </label>
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={podiumRequest(names, note) === null}>
          Set podium
        </button>
        {saved.length > 0 && (
          <button type="button" className="btn-antique" onClick={() => void clear()}>
            Use the results again
          </button>
        )}
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </fieldset>
  );
}
