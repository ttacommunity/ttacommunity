/** A button that opens a prompt to move a season to another tournament. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournamentRow } from '../lib/admin-api';
import { seasonDestinations } from '../lib/tournament-groups';
import Notice from './Notice';
import { SaveStatus } from './SaveBar';
import { useSave } from './useSave';

/** MoveSeasonButton allows moving a tournament season to another tournament, optionally with a new label. */
export default function MoveSeasonButton({
  fromSlug,
  season,
  all,
  onReload,
}: {
  fromSlug: string;
  season: string;
  all: readonly AdminTournamentRow[];
  onReload: () => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [dest, setDest] = useState('');
  const [newLabel, setNewLabel] = useState(season);
  const saving = useSave();

  const destinations = seasonDestinations(all, fromSlug);

  const move = async () => {
    if (!dest) return;
    const trimmed = newLabel.trim();
    const r = await saving.run(() => req.send(req.moveSeason(fromSlug, season, dest, trimmed !== season ? trimmed : undefined)), { done: 'Moved', working: 'Moving…' });
    if (!r?.ok) return;
    await onReload();
    setOpen(false);
  };

  return (
    <span className="move-season">
      {!open ? (
        <button
          type="button"
          className="btn-antique btn-settings"
          onClick={() => {
            setOpen(true);
            setDest('');
            setNewLabel(season);
          }}
          disabled={destinations.length === 0}
        >
          Move season
        </button>
      ) : (
        <span className="move-season-form">
          <select
            className="input-antique"
            value={dest}
            aria-label="Destination tournament"
            onChange={(e) => setDest(e.target.value)}
          >
            <option value="">Move to tournament…</option>
            {destinations.map((d) => (
              <option key={d.slug} value={d.slug}>
                {d.name}
              </option>
            ))}
          </select>{' '}
          <input
            className="input-antique move-season-input"
            value={newLabel}
            maxLength={64}
            placeholder="Season label"
            aria-label="New season label"
            onChange={(e) => setNewLabel(e.target.value)}
          />{' '}
          <button
            type="button"
            className="btn-antique"
            disabled={dest === '' || newLabel.trim() === ''}
            onClick={() => void move()}
          >
            Confirm move
          </button>{' '}
          <button type="button" className="btn-antique" onClick={() => setOpen(false)}>
            Cancel
          </button>
        </span>
      )}{' '}
      <SaveStatus save={saving} />
      <Notice outcome={saving.refusal} onClose={saving.close} />
    </span>
  );
}
