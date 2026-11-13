/** A group's page: its name, type and description, then what it holds, chosen with Modify. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournamentRow, TournamentBrief } from '../lib/admin-api';
import { toggled } from '@ttac/shared/toggle';
import { candidatesFor, groupChange, membersOf } from '../lib/tournament-groups';
import { opensSettings, settingsHref } from '../lib/tournament-page';
import MoveButton from './MoveButton';
import Notice from './Notice';
import { GROUP_KINDS } from './NewGroupForm';
import TournamentTable from './TournamentTable';
import SaveBar, { SaveStatus } from './SaveBar';
import { useSave, type Save } from './useSave';

type Kind = AdminTournamentRow['kind'];

/** GroupSettings edits a group's name, type and description and moves it, open when the address asks for it. */
function GroupSettings({ g, all, saving, onReload }: { g: TournamentBrief; all: readonly AdminTournamentRow[]; saving: Save; onReload: () => Promise<void> }) {
  const [shown, setShown] = useState(() => opensSettings(new URLSearchParams(window.location.search)));
  const [name, setName] = useState(g.name);
  const [kind, setKind] = useState<Kind>(g.kind);
  const [description, setDescription] = useState(g.description);
  const change = groupChange(g, { name, kind, description });
  const save = async () => {
    const r = await saving.run(change === null ? null : () => req.send(req.setTournament(g.slug, change)), { done: 'Saved' });
    if (r?.ok) await onReload();
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
          <SaveBar save={saving}>
            <button type="button" className="btn-antique" onClick={save} disabled={change === null}>
              Save
            </button>
          </SaveBar>
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
  const saving = useSave();
  const start = () => {
    setChosen(members.map((m) => m.slug));
    saving.reset();
    setModifying(true);
  };
  const done = async () => {
    const r = await saving.run(() => req.send(req.setMembers(g.slug, chosen)), { done: 'Saved' });
    if (!r?.ok) return;
    await onReload();
    setModifying(false);
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
            </button> <SaveStatus save={saving} />
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
            <SaveStatus save={saving} prefix={`${chosen.length} chosen`} />
          </p>
          <Notice outcome={saving.refusal} onClose={saving.close} />
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
  // Held here, so the settings' Saved outlives the remount their new values cause.
  const saving = useSave();
  return (
    <>
      <GroupSettings key={`${g.name}-${g.kind}-${g.description}-${g.parent}`} g={g} all={all} saving={saving} onReload={onReload} />
      <Members g={g} all={all} onReload={onReload} />
    </>
  );
}
