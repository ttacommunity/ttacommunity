/** A group's page: its name, type and description, then what it holds, chosen with Modify. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournamentRow, TournamentBrief } from '../lib/admin-api';
import { candidatesFor, groupChange, membersOf, toggled } from '../lib/tournament-groups';
import { opensSettings, settingsHref } from '../lib/tournament-page';
import MoveButton from './MoveButton';
import { GROUP_KINDS } from './NewGroupForm';
import TournamentTable from './TournamentTable';

type Kind = AdminTournamentRow['kind'];

/** GroupSettings edits a group's name, type and description and moves it, open when the address asks for it. */
function GroupSettings({ g, all, onReload }: { g: TournamentBrief; all: readonly AdminTournamentRow[]; onReload: () => Promise<void> }) {
  const [shown, setShown] = useState(() => opensSettings(new URLSearchParams(window.location.search)));
  const [name, setName] = useState(g.name);
  const [kind, setKind] = useState<Kind>(g.kind);
  const [description, setDescription] = useState(g.description);
  const [status, setStatus] = useState('');
  const change = groupChange(g, { name, kind, description });
  const save = async () => {
    setStatus('Saving…');
    if (change === null) return;
    const r = await req.send(req.setTournament(g.slug, change));
    if (!r.ok) {
      setStatus(r.message);
      return;
    }
    await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="group-settings-heading">
      <h2 id="group-settings-heading" className="rating-card-heading">
        <button type="button" className="btn-antique" onClick={() => setShown(!shown)} aria-expanded={shown}>
          Settings
        </button>
      </h2>
      {shown && (
        <>
          <div className="setting">
            <label htmlFor="group-name">Name</label>{' '}
            <input className="input-antique" id="group-name" value={name} maxLength={100} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="setting">
            <label htmlFor="group-kind">It holds</label>{' '}
            <select className="input-antique" id="group-kind" value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
              {GROUP_KINDS.map((k) => (
                <option key={k.kind} value={k.kind}>
                  {k.label}
                </option>
              ))}
            </select>
          </div>
          <div className="note-box">
            <label htmlFor="group-description">Description</label>
            <textarea className="input-antique" id="group-description" value={description} maxLength={2000} rows={2} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <p>
            <button type="button" className="btn-antique" onClick={save} disabled={change === null}>
              Save
            </button>{' '}
            <span className="note-status" role="status">
              {status}
            </span>
          </p>
          <p>
            <MoveButton t={g} all={all} onReload={onReload} />
          </p>
        </>
      )}
    </section>
  );
}

/** Members lists what the group holds, each with its settings; Modify picks them from what matches its type. */
function Members({ g, all, onReload }: { g: TournamentBrief; all: readonly AdminTournamentRow[]; onReload: () => Promise<void> }) {
  const members = membersOf(all, g.slug);
  const [modifying, setModifying] = useState(false);
  const [chosen, setChosen] = useState<string[]>([]);
  const [everyType, setEveryType] = useState(false);
  const [status, setStatus] = useState('');
  const start = () => {
    setChosen(members.map((m) => m.slug));
    setStatus('');
    setModifying(true);
  };
  const done = async () => {
    setStatus('Saving…');
    const r = await req.send(req.setMembers(g.slug, chosen));
    if (!r.ok) {
      setStatus(r.message);
      return;
    }
    await onReload();
    setModifying(false);
    setStatus('Saved');
  };
  const toggle = (slug: string) => setChosen((c) => toggled(c, slug));
  return (
    <section className="rating-card admin-card" aria-labelledby="members-heading">
      <h2 id="members-heading" className="rating-card-heading">
        In this group
      </h2>
      {!modifying && (
        <>
          <p>
            <button type="button" className="btn-antique" onClick={start}>
              Modify
            </button>{' '}
            <span className="note-status" role="status">
              {status}
            </span>
          </p>
          {members.length === 0 ? (
            <p className="muted">Nothing yet. Modify adds tournaments and groups.</p>
          ) : (
            <TournamentTable
              rows={members}
              all={all}
              caption={`What ${g.name} holds`}
              action={(t) => (
                <a className="btn-antique btn-settings" href={settingsHref(t.slug, t.is_group)} aria-label={`Settings for ${t.name}`}>
                  Settings
                </a>
              )}
            />
          )}
        </>
      )}
      {modifying && (
        <>
          <p>
            <button type="button" className="btn-antique" onClick={done}>
              Done
            </button>{' '}
            <button type="button" className="btn-antique" onClick={() => setModifying(false)}>
              Cancel
            </button>{' '}
            <label className="group-member">
              <input type="checkbox" checked={everyType} onChange={(e) => setEveryType(e.target.checked)} /> Show every type
            </label>{' '}
            <span className="note-status" role="status">
              {chosen.length} chosen{status !== '' && ` · ${status}`}
            </span>
          </p>
          <TournamentTable
            rows={candidatesFor(all, g.slug, everyType)}
            all={all}
            details
            caption={`What ${g.name} can hold`}
            action={(t) => (
              <button
                type="button"
                className="btn-antique"
                aria-pressed={chosen.includes(t.slug)}
                aria-label={`${chosen.includes(t.slug) ? 'Remove' : 'Add'} ${t.name}`}
                onClick={() => toggle(t.slug)}
              >
                {chosen.includes(t.slug) ? 'Remove' : 'Add'}
              </button>
            )}
          />
        </>
      )}
    </section>
  );
}

/** GroupPage shows a group's settings at the top, then what it holds. */
export default function GroupPage({ g, all, onReload }: { g: TournamentBrief; all: readonly AdminTournamentRow[]; onReload: () => Promise<void> }) {
  return (
    <>
      <GroupSettings key={`${g.name}-${g.kind}-${g.description}-${g.parent}`} g={g} all={all} onReload={onReload} />
      <Members g={g} all={all} onReload={onReload} />
    </>
  );
}
