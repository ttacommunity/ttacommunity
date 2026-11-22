/** The sheet that edits a Discord command's settings: who sees its reply, the format it shows when its asker gives none, and how many rows its list shows. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import * as req from '../lib/admin-requests';
import { FORMATS, REPLIES, ROWS, commandChanged } from '../lib/discord-bot';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Command = components['schemas']['DiscordBotCommand'];

interface Props {
  command: Command;
  opener: HTMLElement;
  /** Runs once the settings are saved. */
  onSaved: () => void;
  onClose: () => void;
}

export default function CommandSheet({ command, opener, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [draft, setDraft] = useState<Command>(command);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = `command-${command.name}`;
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.setCommand(command.name, { on: draft.on, reply: draft.reply, format: draft.format ?? undefined, results: draft.results ?? undefined }));
    if (!r.ok) return setOutcome(refused(r.message, id));
    onSaved();
    dialog.current?.close();
  };
  return (
    <Sheet
      dialog={dialog}
      id={id}
      title={`/${command.name}`}
      opener={opener}
      onClose={onClose}
      actions={
        <>
          <button type="button" className="btn-quiet" onClick={() => dialog.current?.close()}>
            Cancel
          </button>
          <button type="submit" form={`${id}-form`} className="btn-antique" disabled={!commandChanged(command, draft)}>
            Save
          </button>
        </>
      }
    >
      <form id={`${id}-form`} className="sheet-body" onSubmit={(e) => void save(e)}>
        <p className="muted">{command.description}</p>
        <div className="admin-add">
          <label>
            Who sees a reply
            <select className="input-antique" value={draft.reply} onChange={(e) => setDraft({ ...draft, reply: e.target.value as Command['reply'] })}>
              {REPLIES.map((r) => (
                <option key={r.value} value={r.value}>
                  {r.label}
                </option>
              ))}
            </select>
          </label>
          {draft.format !== undefined && draft.format !== null && (
            <label>
              Format when none is given
              <select className="input-antique" value={String(draft.format)} onChange={(e) => setDraft({ ...draft, format: Number(e.target.value) })}>
                {FORMATS.map((f) => (
                  <option key={f.value} value={f.value}>
                    {f.label}
                  </option>
                ))}
              </select>
            </label>
          )}
          {draft.results !== undefined && draft.results !== null && (
            <label>
              Rows shown
              <select className="input-antique" value={String(draft.results)} onChange={(e) => setDraft({ ...draft, results: Number(e.target.value) })}>
                {ROWS.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
            </label>
          )}
        </div>
        <Notice outcome={outcome} onClose={() => setOutcome(null)} />
      </form>
    </Sheet>
  );
}
