/** The Members tab: the community's people, listed and searched, added and edited by admins and organisers; banned, unbanned and removed by admins. */
import { useEffect, useState, type FormEvent } from 'react';
import { offsetLabel } from '@ttac/shared/blitz';
import { adminRequest, type AdminMe, type AdminMember, type Load, type MemberAdded, type MemberChange } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { countryNames, type Country } from '../lib/countries';
import { membersMatching } from '../lib/member-filter';
import { memberToAdd, type PersonDetails } from '../lib/person-form';
import type { Region } from '../lib/zones';
import CountryPick from './CountryPick';
import PersonForm from './PersonForm';
import ZonePick from './ZonePick';

type Draft = { name: string; discord_name: string; email: string; country: string; zone: string; cge_user: string };

/** draftOf fills the edit form from a member. */
function draftOf(m: AdminMember): Draft {
  return { name: m.name, discord_name: m.discord_name, email: m.email, country: m.country ?? '', zone: m.zone, cge_user: m.cge_user === null ? '' : String(m.cge_user) };
}

export default function MembersPanel({ role }: { role: AdminMe['role'] }) {
  const [members, setMembers] = useState<Load<AdminMember[]>>({ kind: 'loading' });
  const [message, setMessage] = useState('');
  const [editing, setEditing] = useState<{ id: number } & Draft | null>(null);
  const [regions, setRegions] = useState<Region[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  const [search, setSearch] = useState('');
  const shown = members.kind === 'ready' ? membersMatching(members.data, search) : [];
  const load = async () => {
    const r = await adminRequest<AdminMember[]>('/api/members', {});
    setMembers(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void load();
    void adminRequest<Region[]>('/api/regions', {}).then((r) => (r.ok ? setRegions(r.data) : setMessage(r.message)));
    void adminRequest<string[]>('/api/countries', {}).then((r) => (r.ok ? setCountries(countryNames(r.data)) : setMessage(r.message)));
  }, []);
  const run = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    setMessage(res.ok ? done : res.message);
    await load();
    return res.ok;
  };
  const add = async (details: PersonDetails) => {
    const r = await req.send<MemberAdded>(req.addMember(memberToAdd(details)));
    setMessage(r.ok ? `Added ${details.name.trim()}.` : r.message);
    await load();
    return r.ok;
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (editing === null) return;
    const change: MemberChange = { name: editing.name, discord_name: editing.discord_name, email: editing.email, country: editing.country, zone: editing.zone };
    if (editing.cge_user.trim() !== '') change.cge_user = Number(editing.cge_user);
    if (await run(req.setMember(editing.id, change), 'Saved.')) setEditing(null);
  };
  const ban = (m: AdminMember) => {
    const reason = window.prompt(`Ban ${m.name} from taking part. The reason:`);
    if (reason === null || reason.trim() === '') return;
    void run(req.banMember(m.id, reason), `${m.name} is banned.`);
  };
  const remove = (m: AdminMember) => {
    if (window.confirm(`Remove ${m.name}? Only for a mistaken entry; a member in an edition cannot be removed.`)) void run(req.removeMember(m.id), `Removed ${m.name}.`);
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="members-heading">
      <h2 id="members-heading" className="rating-card-heading">
        Members
      </h2>
      <p className="muted">
        Everyone who takes part in anything we run, by their TTA username as in the app. A name the ratings or a CGE
        account know is linked to them, and the link fills itself in as games are loaded. A banned member cannot be
        entered in an edition; their games and ratings stay. Organisers reach players by Discord username or email.
      </p>
      {message !== '' && (
        <p className="admin-message" role="status">
          {message}
        </p>
      )}
      <div className="admin-subsection">
        <h3>Add a member</h3>
        <PersonForm searchBase="/api/people" taken={{ entered: [], membersTaken: true }} regions={regions} countries={countries} submitLabel="Add" onSubmit={add} refresh={members} />
      </div>
      <div className="admin-subsection">
        <h3>Every member</h3>
        {members.kind === 'loading' && <p className="muted">Loading…</p>}
        {members.kind === 'error' && (
          <p className="row-error" role="alert">
            {members.message}
          </p>
        )}
        {members.kind === 'ready' && members.data.length === 0 && <p className="muted">No members yet.</p>}
        {members.kind === 'ready' && members.data.length > 0 && (
          <div className="admin-add member-search">
            <label>
              Search
              <input
                className="input-antique"
                type="search"
                value={search}
                placeholder="Name, Discord, email or country"
                title="Separate several names with commas."
                autoComplete="off"
                onChange={(e) => setSearch(e.target.value)}
              />
            </label>
            {shown.length < members.data.length && (
              <span className="muted pick-note" role="status">
                {shown.length} of {members.data.length} members
              </span>
            )}
          </div>
        )}
        {members.kind === 'ready' && members.data.length > 0 && shown.length === 0 && <p className="muted">Nobody matches.</p>}
        {members.kind === 'ready' && shown.length > 0 && (
          <div className="table-responsive">
            <table className="leaderboard-table admin-table">
              <caption className="visually-hidden">{shown.length < members.data.length ? 'The members matching the search' : 'Every member'}</caption>
              <thead>
                <tr>
                  <th scope="col">Username</th>
                  <th scope="col">CGE id</th>
                  <th scope="col">Discord</th>
                  <th scope="col">Email</th>
                  <th scope="col">Country</th>
                  <th scope="col">Time zone</th>
                  <th scope="col">In</th>
                  <th scope="col">
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {shown.map((m) =>
                  editing?.id === m.id ? (
                    <tr key={m.id}>
                      <th scope="row">
                        <input className="input-antique" aria-label={`Username of ${m.name}`} value={editing.name} maxLength={100} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
                      </th>
                      <td>
                        <input className="input-antique" aria-label={`CGE id of ${m.name}`} inputMode="numeric" value={editing.cge_user} onChange={(e) => setEditing({ ...editing, cge_user: e.target.value })} />
                      </td>
                      <td>
                        <input className="input-antique" aria-label={`Discord username of ${m.name}`} value={editing.discord_name} maxLength={100} onChange={(e) => setEditing({ ...editing, discord_name: e.target.value })} />
                      </td>
                      <td>
                        <input className="input-antique" aria-label={`Email of ${m.name}`} type="email" value={editing.email} maxLength={254} onChange={(e) => setEditing({ ...editing, email: e.target.value })} />
                      </td>
                      <td>
                        <CountryPick label={`Country of ${m.name}`} hideLabel countries={countries} value={editing.country} onChange={(country) => setEditing({ ...editing, country })} />
                      </td>
                      <td>
                        <ZonePick label={`Time zone of ${m.name}`} hideLabel regions={regions} value={editing.zone} onChange={(zone) => setEditing({ ...editing, zone })} />
                      </td>
                      <td>{m.entries}</td>
                      <td className="run-buttons">
                        <button type="button" className="btn-antique" onClick={(e) => void save(e)}>
                          Save
                        </button>
                        <button type="button" className="btn-antique" onClick={() => setEditing(null)}>
                          Cancel
                        </button>
                      </td>
                    </tr>
                  ) : (
                    <tr key={m.id} className={m.banned ? 'row-error' : ''}>
                      <th scope="row">
                        {m.name}
                        {m.player !== '' && m.player !== m.name && <span className="row-detail muted">{m.player} in the ratings</span>}
                        {m.player === '' && <span className="row-detail muted">not in the ratings</span>}
                        {m.banned && <span className="row-detail">banned: {m.ban_reason}</span>}
                      </th>
                      <td>{m.cge_user ?? ''}</td>
                      <td>{m.discord_name}</td>
                      <td>{m.email}</td>
                      <td>{m.country ?? ''}</td>
                      <td>
                        {m.zone} <span className="muted">{offsetLabel(m.utc_offset)}</span>
                      </td>
                      <td>{m.entries}</td>
                      <td className="run-buttons">
                        <button type="button" className="btn-antique" onClick={() => setEditing({ id: m.id, ...draftOf(m) })}>
                          Edit
                        </button>
                        {role === 'admin' && !m.banned && (
                          <button type="button" className="btn-antique" onClick={() => ban(m)}>
                            Ban
                          </button>
                        )}
                        {role === 'admin' && m.banned && (
                          <button type="button" className="btn-antique" onClick={() => void run(req.unbanMember(m.id), `${m.name} is no longer banned.`)}>
                            Unban
                          </button>
                        )}
                        {role === 'admin' && (
                          <button type="button" className="btn-antique" onClick={() => remove(m)}>
                            Remove
                          </button>
                        )}
                      </td>
                    </tr>
                  ),
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}
