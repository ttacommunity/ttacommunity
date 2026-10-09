/** A single Move button that opens a list of groups, and moves a tournament or group into the one chosen. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournamentRow, TournamentBrief } from '../lib/admin-api';
import { moveChoices } from '../lib/tournament-groups';

/** MoveButton moves t into a group from all, or out of every group, then calls onReload. */
export default function MoveButton({
  t,
  all,
  onReload,
}: {
  t: TournamentBrief;
  all: readonly AdminTournamentRow[];
  onReload: () => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState('');
  const choices = moveChoices(all, t.slug);
  const move = async (parent: string) => {
    setStatus('Moving…');
    const r = await req.send(req.moveInto(t.slug, parent));
    if (!r.ok) {
      setStatus(r.message);
      return;
    }
    await onReload();
    setOpen(false);
    setStatus('Moved');
  };
  return (
    <span className="move-button">
      {!open ? (
        <button type="button" className="btn-antique" onClick={() => setOpen(true)} disabled={choices.length === 0}>
          Move
        </button>
      ) : (
        <>
          <label htmlFor={`move-${t.slug}`} className="visually-hidden">
            Move {t.name} to
          </label>
          <select
            className="input-antique"
            id={`move-${t.slug}`}
            value=""
            onChange={(e) => e.target.value !== '' && void move(choices[Number(e.target.value)].parent)}
          >
            <option value="">Move to…</option>
            {choices.map((c, i) => (
              <option key={c.parent || '(top)'} value={i}>
                {c.label}
              </option>
            ))}
          </select>{' '}
          <button type="button" className="btn-antique" onClick={() => setOpen(false)}>
            Cancel
          </button>
        </>
      )}{' '}
      <span className="note-status" role="status">
        {status}
      </span>
    </span>
  );
}
