/** The sheet that changes a game: add one to a group, remove one, or replace one with a new game, each with its reason. */
import { useRef, useState, type FormEvent } from 'react';
import Sheet, { SheetSection } from '@ttac/shared/Sheet';
import Segmented from '@ttac/shared/Segmented';
import * as req from '../lib/admin-requests';
import { ACTIONS, changeBody, emptyEntry, entryFrom, type Action, type ChangeableGame, type EntryForm } from '../lib/game-changes';
import { refused, type Outcome } from '../lib/outcome';
import GameEntryFields from './GameEntryFields';
import GameFind from './GameFind';
import GroupPick, { type Group } from './GroupPick';
import Notice from './Notice';

interface Props {
  opener: HTMLElement;
  /** Runs once the change is made, with what to say about it. */
  onSaved: (message: string) => void;
  onClose: () => void;
}

const DONE: Record<Action, string> = {
  add: 'Added the game. It is rated from the next run.',
  remove: 'Removed the game. It is kept here, and left out from the next run.',
  replace: 'Replaced the game. The new game is rated from the next run.',
};

export default function GameChangeSheet({ opener, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [action, setAction] = useState<Action>('add');
  const [found, setFound] = useState<ChangeableGame | null>(null);
  const [group, setGroup] = useState<Group | null>(null);
  const [entry, setEntry] = useState<EntryForm>(emptyEntry());
  const [reason, setReason] = useState('');
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = 'game-change';
  const failed = (message: string) => setOutcome(refused(message, id));
  const find = (g: ChangeableGame | null) => {
    setFound(g);
    setOutcome(null);
    if (g !== null) setEntry(entryFrom(g));
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const b = changeBody(action, { game: found?.id, group: group?.id, entry, reason });
    if (!b.ok) return failed(b.message);
    const r = await req.send(req.makeGameChange(b.body));
    if (!r.ok) return failed(r.message);
    onSaved(DONE[action]);
    dialog.current?.close();
  };

  return (
    <Sheet
      dialog={dialog}
      id={id}
      title="Change a game"
      opener={opener}
      onClose={onClose}
      actions={
        <>
          <button type="button" className="btn-quiet" onClick={() => dialog.current?.close()}>
            Cancel
          </button>
          <button type="submit" form={`${id}-form`} className="btn-antique">
            {ACTIONS.find((a) => a.value === action)?.label}
          </button>
        </>
      }
    >
      <form id={`${id}-form`} className="sheet-body" onSubmit={(e) => void save(e)}>
        <Segmented<Action> label="Change" options={ACTIONS} value={action} onChange={setAction} />
        {action === 'add' ? (
          <SheetSection title="Where">
            <GroupPick onPick={setGroup} onFailed={failed} />
          </SheetSection>
        ) : (
          <SheetSection title={action === 'remove' ? 'The game to remove' : 'The game to replace'}>
            <GameFind found={found} onFound={find} onFailed={failed} />
          </SheetSection>
        )}
        {action !== 'remove' && (
          <SheetSection title="The new game">
            <GameEntryFields entry={entry} onChange={setEntry} next={action === 'add' ? (group?.next ?? null) : null} />
          </SheetSection>
        )}
        <SheetSection title="Why">
          <div className="field-row">
            <label>
              Reason (shown in admin only)
              <textarea className="input-antique" value={reason} maxLength={1000} rows={2} required onChange={(e) => setReason(e.target.value)} />
            </label>
          </div>
        </SheetSection>
        <Notice outcome={outcome} onClose={() => setOutcome(null)} />
      </form>
    </Sheet>
  );
}
