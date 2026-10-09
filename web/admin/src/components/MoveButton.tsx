/** A single Move button that opens a list of groups, and moves a tournament or group into the one chosen. */
import { useState } from 'react';
import { adminRequest, type AdminTournament } from '../lib/admin-api';
import { groupsToJoin } from '../lib/tournament-groups';

/** MoveButton moves t into a group from all, or out of every group, then calls onReload. */
export default function MoveButton({
  t,
  all,
  onReload,
}: {
  t: AdminTournament;
  all: readonly AdminTournament[];
  onReload: () => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState('');
  const groups = groupsToJoin(all, t.slug);
  const move = async (parent: string) => {
    setStatus('Moving…');
    const r = await adminRequest(`/api/tournaments/${t.slug}`, { method: 'PATCH', json: { parent } });
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
        <button type="button" className="btn-antique" onClick={() => setOpen(true)} disabled={groups.length === 0 && t.parent === ''}>
          Move
        </button>
      ) : (
        <>
          <label htmlFor={`move-${t.slug}`} className="visually-hidden">
            Move {t.name} to
          </label>
          <select className="input-antique" id={`move-${t.slug}`} value="" onChange={(e) => void move(e.target.value === '-' ? '' : e.target.value)}>
            <option value="">Move to…</option>
            {t.parent !== '' && <option value="-">No group: the top level</option>}
            {groups.map((g) => (
              <option key={g.slug} value={g.slug}>
                {g.name}
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
