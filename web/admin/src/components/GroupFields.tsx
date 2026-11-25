/** A group's Name and what it holds, shared by the new-group form and a group's settings. */
import { useId } from 'react';
import type { AdminTournamentRow } from '../lib/admin-api';

type Kind = AdminTournamentRow['kind'];

const GROUP_KINDS: { kind: Kind; label: string }[] = [
  { kind: 'series', label: 'Series: editions of one event' },
  { kind: 'cup', label: 'Cups: each tournament one event' },
  { kind: 'league', label: 'Leagues: seasons in one competition' },
  { kind: 'ladder', label: 'Ladders' },
  { kind: 'group', label: 'Groups: a group of groups and tournaments' },
];

/** GroupFields edits a group's name and kind, calling onName and onKind as they change. */
export default function GroupFields({ name, kind, onName, onKind }: { name: string; kind: Kind; onName: (name: string) => void; onKind: (kind: Kind) => void }) {
  const id = useId();
  return (
    <>
      <div className="setting">
        <label htmlFor={`${id}-name`}>Name</label>{' '}
        <input className="input-antique" id={`${id}-name`} value={name} maxLength={100} required onChange={(e) => onName(e.target.value)} />
      </div>
      <div className="setting">
        <label htmlFor={`${id}-kind`}>It holds</label>{' '}
        <select className="input-antique" id={`${id}-kind`} value={kind} onChange={(e) => onKind(e.target.value as Kind)}>
          {GROUP_KINDS.map((k) => (
            <option key={k.kind} value={k.kind}>
              {k.label}
            </option>
          ))}
        </select>
      </div>
    </>
  );
}
