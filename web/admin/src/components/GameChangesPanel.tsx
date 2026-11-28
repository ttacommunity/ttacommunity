/** Games added, removed or replaced in admin: each change with its reason and who made it, to edit or delete, and the sheet that makes a new one. */
import { useEffect, useState } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import * as req from '../lib/admin-requests';
import { changeRows, editStart, type ChangeableGame, type ChangeForm, type ChangeRow, type GameChange } from '../lib/game-changes';
import { loadFailure, outcomeOf, shownAt, succeeded, type Outcome } from '../lib/outcome';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';
import GameChangeSheet from './GameChangeSheet';
import Notice from './Notice';

/** GameChangesPanel lists the changes, newest first, each to edit or delete, and opens the sheet that makes one. */
export default function GameChangesPanel() {
  const [changes, setChanges] = useState<GameChange[] | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [changing, setChanging] = useState<{ opener: HTMLElement; editing?: { id: number; form: ChangeForm } } | null>(null);
  const dialog = useDialog();
  const zone = viewerZone();
  const at = 'game-changes';

  const load = async () => {
    const r = await req.send<GameChange[]>(req.gameChanges);
    if (r.ok) setChanges(r.data);
    else setOutcome(loadFailure(r.message, at));
  };
  useEffect(() => {
    void load();
  }, []);

  const remove = async (c: ChangeRow, opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: 'Delete this change?', text: c.undo, action: 'Delete', final: true }, opener);
    if (!sure) return;
    const r = await req.send(req.undoGameChange(c.id));
    setOutcome(outcomeOf(r, 'Deleted the change. The next run rates the games as they are now.', at));
    await load();
  };
  const edit = async (c: GameChange, opener: HTMLElement) => {
    const now = async (code: string | undefined): Promise<ChangeableGame | null> => {
      if (code === undefined || code === '') return null;
      const r = await req.send<ChangeableGame>(req.gameByCode(code));
      return r.ok ? r.data : null;
    };
    const [original, added] = await Promise.all([now(c.game?.code), now(c.added?.code)]);
    setChanging({ opener, editing: { id: c.id, form: editStart(c, original, added) } });
  };
  const rows = changeRows(changes ?? [], (iso) => formatInstant(iso, zone));

  return (
    <AdminCard
      id="game-changes-heading"
      title="Game changes"
      actions={
        <button type="button" className="btn-antique" onClick={(e) => setChanging({ opener: e.currentTarget })}>
          Change a game
        </button>
      }
    >
      {dialog.element}
      <p className="muted">
        Add a game CGE does not have, remove one that should not count, or replace one with the game that should. A removed or replaced game is kept,
        never rated, and every CGE extract keeps the change. Ratings move at the next run.
      </p>
      <Notice outcome={shownAt(outcome, at)} onClose={() => setOutcome(null)} />
      {changes !== null && rows.length === 0 && <p className="muted">No games changed yet.</p>}
      {rows.length > 0 && (
        <AdminTable caption="Games changed in admin, newest first">
          <thead>
            <tr>
              <th scope="col">When</th>
              <th scope="col">Change</th>
              <th scope="col">Game</th>
              <th scope="col">New game</th>
              <th scope="col">Reason</th>
              <th scope="col">By</th>
              <th scope="col">Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id}>
                <td>{r.when}</td>
                <td>{r.action}</td>
                <td>{r.game}</td>
                <td>{r.added}</td>
                <td>{r.reason}</td>
                <td>{r.by}</td>
                <td>
                  <button type="button" className="btn-antique btn-compact" onClick={(e) => void edit(r.change, e.currentTarget)}>
                    Edit
                  </button>
                  <button type="button" className="btn-antique btn-danger btn-compact" onClick={(e) => void remove(r, e.currentTarget)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </AdminTable>
      )}
      {changing !== null && (
        <GameChangeSheet
          opener={changing.opener}
          editing={changing.editing}
          onSaved={(message) => {
            setOutcome(succeeded(message, at));
            void load();
          }}
          onClose={() => setChanging(null)}
        />
      )}
    </AdminCard>
  );
}
