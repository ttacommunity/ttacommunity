/** A form that makes a group of related tournaments from its name and type, then opens its page. */
import { useState, type FormEvent } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournamentRow } from '../lib/admin-api';
import { newGroupBody } from '../lib/tournament-groups';
import { settingsHref } from '../lib/tournament-page';
import GroupFields from './GroupFields';
import SaveBar from './SaveBar';
import { useSave } from './useSave';

type Kind = AdminTournamentRow['kind'];

/** NewGroupForm makes an empty group and goes to its page, where its members are chosen. */
export default function NewGroupForm() {
  const [name, setName] = useState('');
  const [kind, setKind] = useState<Kind>('series');
  const saving = useSave();
  const make = async (e: FormEvent) => {
    e.preventDefault();
    const body = newGroupBody(name, kind);
    if (body === null) return;
    const r = await saving.run(() => req.send<{ slug: string }>(req.newGroup(body)), { done: 'Made', working: 'Making…' });
    if (r?.ok) window.location.href = settingsHref(r.data.slug);
  };
  return (
    <form className="new-group" onSubmit={make} aria-labelledby="new-group-heading">
      <h3 id="new-group-heading">New group</h3>
      <GroupFields name={name} kind={kind} onName={setName} onKind={setKind} />
      <SaveBar save={saving}>
        <button type="submit" className="btn-antique" disabled={newGroupBody(name, kind) === null}>
          Make the group
        </button>
      </SaveBar>
    </form>
  );
}
