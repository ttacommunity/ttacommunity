/** The Challenges tab: the entries by view, searched as you type; each one corrected, checked, marked bugged, hidden or shown; and a name merged into another or a member. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import Segmented from '@ttac/shared/Segmented';
import { formatTime, viewerZone } from '@ttac/shared/time';
import type { Load } from '@ttac/shared/load';
import { adminRequest } from '../lib/admin-api';
import { entriesMatching, entriesPath, viewLabel, type View } from '../lib/challenge-entries';
import { LoadNotice } from './Notice';
import './ChallengesPanel.css';

type AdminChallengeEntries = components['schemas']['AdminChallengeEntries'];
type AdminChallengeEntry = components['schemas']['AdminChallengeEntry'];
type Change = components['schemas']['AdminChallengeEntryChange'];
type Merged = components['schemas']['AdminChallengeNameMerged'];

/** ChallengesPanel loads the view asked for, filters it as the search is typed, and reloads it after each change. */
export default function ChallengesPanel() {
  const [view, setView] = useState<View>('all');
  const [search, setSearch] = useState('');
  const [round, setRound] = useState(0);
  const [status, setStatus] = useState<string | null>(null);
  const [entries, setEntries] = useState<Load<AdminChallengeEntries>>({ kind: 'loading' });
  useEffect(() => {
    void adminRequest<AdminChallengeEntries>(entriesPath(view), {}).then((r) =>
      setEntries(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
  }, [view, round]);
  const shown = entries.kind === 'ready' ? entriesMatching(entries.data.entries, search) : [];
  const zone = viewerZone();

  /** change sends a change to entry id and reloads, saying what went wrong if it was refused. */
  const change = async (id: number, c: Change) => {
    const r = await adminRequest<null>(`/api/challenges/entries/${id}`, { method: 'PATCH', json: c });
    setStatus(r.ok ? null : r.message);
    setRound((n) => n + 1);
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="challenge-entries-heading">
      <h2 id="challenge-entries-heading" className="rating-card-heading">
        Challenge entries
      </h2>
      <p className="muted">
        Every domination entered for a challenge, newest first. Times are in {zone}. Every change is in the audit log.
      </p>
      <MergeNames onMerged={() => setRound((n) => n + 1)} />
      {entries.kind === 'ready' && (
        <Segmented
          label="Which entries"
          look="records"
          value={view}
          onChange={setView}
          options={entries.data.counts.map((c) => ({ value: c.view, label: viewLabel(c.view, c.entries) }))}
        />
      )}
      {entries.kind === 'ready' && entries.data.entries.length > 0 && (
        <div className="admin-add member-search">
          <label>
            Search
            <input
              className="input-antique"
              type="search"
              value={search}
              placeholder="Player or challenge"
              title="Separate several with commas."
              autoComplete="off"
              onChange={(e) => setSearch(e.target.value)}
            />
          </label>
          {shown.length < entries.data.entries.length && (
            <span className="muted pick-note" role="status">
              {shown.length} of {entries.data.entries.length} entries
            </span>
          )}
        </div>
      )}
      {status !== null && <LoadNotice message={status} />}
      {entries.kind === 'loading' && <p className="muted">Loading…</p>}
      {entries.kind === 'error' && <LoadNotice message={entries.message} />}
      {entries.kind === 'ready' && entries.data.entries.length > 0 && shown.length === 0 && <p className="muted">No entry matches.</p>}
      {entries.kind === 'ready' && shown.length > 0 && (
        <>
          <div className="table-responsive">
            <table className="leaderboard-table admin-table">
              <caption className="visually-hidden">{shown.length < entries.data.entries.length ? 'The entries matching the search, newest first' : 'Every entry in the view, newest first'}</caption>
              <thead>
                <tr>
                  <th scope="col">Entered</th>
                  <th scope="col">Challenge</th>
                  <th scope="col">Player</th>
                  <th scope="col">Domination</th>
                  <th scope="col">Proof</th>
                  <th scope="col">Marks</th>
                  <th scope="col">Actions</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((e) => (
                  <EntryRow key={e.id} entry={e} zone={zone} onChange={(c) => void change(e.id, c)} />
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}

/** EntryRow is one entry with its marks, what looks wrong, and the changes it can take; a field being edited shows a box. */
function EntryRow({ entry: e, zone, onChange }: { entry: AdminChallengeEntry; zone: string; onChange: (c: Change) => void }) {
  const [editing, setEditing] = useState<'domination' | 'proof' | 'hide' | null>(null);
  const [text, setText] = useState('');
  const edit = (field: 'domination' | 'proof' | 'hide', start: string) => {
    setEditing(field);
    setText(start);
  };
  const save = () => {
    if (editing === 'domination') onChange({ domination: Number(text) });
    if (editing === 'proof') onChange({ proof_url: text });
    if (editing === 'hide') onChange({ hidden: true, hidden_reason: text });
    setEditing(null);
  };
  return (
    <tr>
      <td>{formatTime(e.entered_at, zone)}</td>
      <td>{e.challenge_name}</td>
      <th scope="row">
        {e.player}
        {e.member === null && <span className="row-detail muted">not linked to a member</span>}
      </th>
      <td>{e.domination}</td>
      <td>
        {e.proof_url === null ? (
          <span className="muted">—</span>
        ) : (
          <a href={e.proof_url} target="_blank" rel="noopener noreferrer">
            Proof
          </a>
        )}
      </td>
      <td>
        {e.hidden && `Hidden: ${e.hidden_reason}`}
        {e.flags.map((f) => (
          <span key={f} className="row-detail">
            ⚠ {f}
          </span>
        ))}
      </td>
      <td>
        {editing !== null ? (
          <span className="note-actions">
            <input
              className="input-antique"
              aria-label={editing === 'hide' ? 'Why it is hidden' : editing === 'proof' ? 'Proof link, empty for none' : 'Domination'}
              type={editing === 'domination' ? 'number' : 'text'}
              value={text}
              onChange={(ev) => setText(ev.target.value)}
            />
            <button type="button" className="btn-antique" disabled={editing === 'hide' && text.trim() === ''} onClick={save}>
              Save
            </button>
            <button type="button" className="btn-antique" onClick={() => setEditing(null)}>
              Cancel
            </button>
          </span>
        ) : (
          <span className="note-actions">
            <button type="button" className="btn-antique" onClick={() => edit('domination', String(e.domination))}>
              Domination
            </button>
            <button type="button" className="btn-antique" onClick={() => edit('proof', e.proof_url ?? '')}>
              Proof
            </button>
            {e.proof_url !== null && (
              <button
                type="button"
                className={e.checked ? 'btn-antique challenge-toggle on' : 'btn-antique challenge-toggle'}
                aria-pressed={e.checked}
                title={e.checked ? 'An admin has looked at the proof; press to undo' : 'Mark the proof as looked at'}
                onClick={() => onChange({ checked: !e.checked })}
              >
                Checked
              </button>
            )}
            <button
              type="button"
              className={e.bugged ? 'btn-antique challenge-toggle on' : 'btn-antique challenge-toggle'}
              aria-pressed={e.bugged}
              title={e.bugged ? 'Made through a game bug, never counted; press to undo' : 'Mark as made through a game bug'}
              onClick={() => onChange({ bugged: !e.bugged })}
            >
              Bug
            </button>
            {e.hidden ? (
              <button type="button" className="btn-antique" onClick={() => onChange({ hidden: false })}>
                Show
              </button>
            ) : (
              <button type="button" className="btn-antique btn-danger" onClick={() => edit('hide', '')}>
                Hide
              </button>
            )}
          </span>
        )}
      </td>
    </tr>
  );
}

/** MergeNames moves every entry and weekly placing under one name to the member of another TTA username. */
function MergeNames({ onMerged }: { onMerged: () => void }) {
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [said, setSaid] = useState<string | null>(null);
  const merge = async () => {
    const r = await adminRequest<Merged>('/api/challenges/names', { method: 'POST', json: { from, to } });
    if (!r.ok) {
      setSaid(r.message);
      return;
    }
    setSaid(`Moved ${r.data.moved} ${r.data.moved === 1 ? 'entry' : 'entries'} to the member ${to.trim()}.`);
    setFrom('');
    setTo('');
    onMerged();
  };
  return (
    <div className="admin-subsection">
      <h3>Merge a name</h3>
      <p className="muted">
        Moves every entry and weekly placing under the first name to the member whose TTA username is the second: for a misspelt name, or a name the import
        gave a member of its own. The first name&apos;s member stays, empty, for Members to remove.
      </p>
      <span className="note-actions">
        <input className="input-antique" aria-label="From name" placeholder="From" value={from} onChange={(e) => setFrom(e.target.value)} />
        <span aria-hidden="true">→</span>
        <input className="input-antique" aria-label="To member" placeholder="To" value={to} onChange={(e) => setTo(e.target.value)} />
        <button type="button" className="btn-antique" disabled={from.trim() === '' || to.trim() === ''} onClick={() => void merge()}>
          Merge
        </button>
      </span>
      {said !== null && (
        <p className="note-status" role="status">
          {said}
        </p>
      )}
    </div>
  );
}
