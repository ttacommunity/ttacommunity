/** A form that makes a group of related tournaments from its name and type, then opens its page. */
import { useState, type FormEvent } from 'react';
import { adminRequest, type AdminTournament } from '../lib/admin-api';
import { settingsHref } from '../lib/tournament-page';

type Kind = AdminTournament['kind'];

/** GROUP_KINDS are what a group can hold, as the forms offer them. */
export const GROUP_KINDS: { kind: Kind; label: string }[] = [
  { kind: 'series', label: 'Series: editions of one event' },
  { kind: 'cup', label: 'Cups: each tournament one event' },
  { kind: 'league', label: 'Leagues: seasons in one competition' },
  { kind: 'ladder', label: 'Ladders' },
  { kind: 'group', label: 'Groups: a group of groups and tournaments' },
];

/** NewGroupForm makes an empty group and goes to its page, where its members are chosen. */
export default function NewGroupForm() {
  const [name, setName] = useState('');
  const [kind, setKind] = useState<Kind>('series');
  const [status, setStatus] = useState('');
  const make = async (e: FormEvent) => {
    e.preventDefault();
    setStatus('Making…');
    const r = await adminRequest<{ slug: string }>('/api/tournaments', { method: 'POST', json: { name: name.trim(), kind } });
    if (r.ok) {
      window.location.href = settingsHref(r.data.slug);
    } else {
      setStatus(r.message);
    }
  };
  return (
    <form className="new-group" onSubmit={make} aria-labelledby="new-group-heading">
      <h3 id="new-group-heading">New group</h3>
      <div className="setting">
        <label htmlFor="new-group-name">Name</label>{' '}
        <input className="input-antique" id="new-group-name" value={name} maxLength={100} required onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="setting">
        <label htmlFor="new-group-kind">It holds</label>{' '}
        <select className="input-antique" id="new-group-kind" value={kind} onChange={(e) => setKind(e.target.value as Kind)}>
          {GROUP_KINDS.map((k) => (
            <option key={k.kind} value={k.kind}>
              {k.label}
            </option>
          ))}
        </select>
      </div>
      <p>
        <button type="submit" className="btn-antique" disabled={name.trim() === ''}>
          Make the group
        </button>{' '}
        <span className="note-status" role="status">
          {status}
        </span>
      </p>
    </form>
  );
}
