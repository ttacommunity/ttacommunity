/** The Members tab: the community's people, listed and searched, added and edited by admins and organisers; banned, unbanned and removed by admins. Players' sign-ins sit with them: requests to be linked first, each member's linked sign-in, then those linked to no one. */
import { Fragment, useEffect, useState, type FormEvent } from 'react';
import { offsetLabel } from '@ttac/shared/blitz';
import { useDialog } from '@ttac/shared/Dialog';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import { adminRequest, type AdminAccount, type AdminMe, type AdminMember, type Load, type MemberAdded, type MemberChange } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { countryNames, type Country } from '../lib/countries';
import { DEFAULT_STATUSES, MEMBER_STATUSES, membersMatching, membersWithStatus, type MemberStatus } from '../lib/member-filter';
import { loadFailure, outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import { memberToAdd, type PersonDetails } from '../lib/person-form';
import { howLinked, signInGroups, who } from '../lib/sign-ins';
import type { Region } from '../lib/zones';
import CountryPick from './CountryPick';
import Notice, { LoadNotice, NoticeRow } from './Notice';
import PersonForm from './PersonForm';
import { UnlinkedSignIns, WaitingRequests } from './SignIns';
import ZonePick from './ZonePick';

type Draft = { name: string; discord_name: string; email: string; country: string; zone: string; cge_user: string };

/** draftOf fills the edit form from a member. */
function draftOf(m: AdminMember): Draft {
  return { name: m.name, discord_name: m.discord_name, email: m.email, country: m.country ?? '', zone: m.zone, cge_user: m.cge_user === null ? '' : String(m.cge_user) };
}

/** SignedInAs shows the account linked to a member: its ways in, how it was linked and when it was last used. */
function SignedInAs({ login }: { login: AdminAccount | undefined }) {
  if (login === undefined) return <span className="muted">—</span>;
  return (
    <>
      {who(login)}
      <span className="row-detail muted">
        {howLinked(login)} · last in {formatInstant(login.signed_in_at, viewerZone())}
      </span>
    </>
  );
}

type Props = { role: AdminMe['role']; accounts: Load<AdminAccount[]>; reloadAccounts: () => Promise<void> };

/** MembersPanel lists the members with their players' accounts, loaded once for the tab's count too. */
export default function MembersPanel({ role, accounts, reloadAccounts }: Props) {
  const [members, setMembers] = useState<Load<AdminMember[]>>({ kind: 'loading' });
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const close = () => setOutcome(null);
  const dialog = useDialog();
  const [editing, setEditing] = useState<{ id: number } & Draft | null>(null);
  const [regions, setRegions] = useState<Region[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  const [search, setSearch] = useState('');
  const [statuses, setStatuses] = useState<ReadonlySet<MemberStatus>>(DEFAULT_STATUSES);
  const signIns = signInGroups(accounts.kind === 'ready' ? accounts.data : []);
  const listed = members.kind === 'ready' ? membersWithStatus(members.data, statuses, new Set(signIns.byMember.keys())) : [];
  const shown = membersMatching(listed, search);
  const toggle = (s: MemberStatus) =>
    setStatuses((prev) => {
      const next = new Set(prev);
      if (!next.delete(s)) next.add(s);
      return next;
    });
  const load = async () => {
    const r = await adminRequest<AdminMember[]>('/api/members', {});
    setMembers(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void load();
    void adminRequest<Region[]>('/api/regions', {}).then((r) => (r.ok ? setRegions(r.data) : setOutcome(loadFailure(r.message, 'add member'))));
    void adminRequest<string[]>('/api/countries', {}).then((r) => (r.ok ? setCountries(countryNames(r.data)) : setOutcome(loadFailure(r.message, 'add member'))));
  }, []);
  // after reloads what answering a request or unlinking changes: the sign-ins, and a member added or given details.
  const after = async () => {
    await Promise.all([load(), reloadAccounts()]);
  };
  const unlink = async (m: AdminMember) => {
    const l = signIns.byMember.get(m.id);
    if (l === undefined) return;
    const sure = await dialog.confirm({
      heading: `Unlink ${who(l)} from ${m.name}?`,
      text: 'They stay signed in, unlinked, and are not linked by Discord name or email again.',
      action: 'Unlink',
    });
    if (!sure) return;
    setOutcome(outcomeOf(await req.send(req.unlinkAccount(l.id)), `Unlinked ${who(l)} from ${m.name}.`, `member ${m.id}`));
    await after();
  };
  const run = async (r: req.Request, done: string, at: string, doneAt = at) => {
    const res = await req.send(r);
    setOutcome(outcomeOf(res, done, at, doneAt));
    await load();
    return res.ok;
  };
  const add = async (details: PersonDetails) => {
    const r = await req.send<MemberAdded>(req.addMember(memberToAdd(details)));
    setOutcome(outcomeOf(r, `Added ${details.name.trim()}.`, 'add member'));
    await load();
    return r.ok;
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (editing === null) return;
    const change: MemberChange = { name: editing.name, discord_name: editing.discord_name, email: editing.email, country: editing.country, zone: editing.zone };
    if (editing.cge_user.trim() !== '') change.cge_user = Number(editing.cge_user);
    if (await run(req.setMember(editing.id, change), 'Saved.', `member ${editing.id}`)) setEditing(null);
  };
  const ban = async (m: AdminMember) => {
    const reason = await dialog.ask({
      heading: `Ban ${m.name}?`,
      text: 'A banned member cannot be entered in an edition; their games and ratings stay.',
      field: 'The reason',
      required: true,
      maxLength: 1000,
      action: 'Ban',
    });
    if (reason !== null) await run(req.banMember(m.id, reason), `${m.name} is banned.`, `member ${m.id}`);
  };
  const remove = async (m: AdminMember) => {
    const sure = await dialog.confirm({
      heading: `Remove ${m.name}?`,
      text: 'Only for a mistaken entry: a member in an edition cannot be removed.',
      action: 'Remove',
      final: true,
    });
    if (sure) await run(req.removeMember(m.id), `Removed ${m.name}.`, `member ${m.id}`, 'members');
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="members-heading">
      {dialog.element}
      <h2 id="members-heading" className="rating-card-heading">
        Members
      </h2>
      <p className="muted">
        Everyone who takes part in anything we run, by their TTA username as in the app. A name the ratings or a CGE
        account know is linked to them, and the link fills itself in as games are loaded. A banned member cannot be
        entered in an edition; their games and ratings stay. Organisers reach players by Discord username or email.
        Players sign in on the site with Discord and ask to be linked to their username: one whose Discord username is
        the Discord name recorded for exactly one member is linked at once; the rest wait at the top of this page.
      </p>
      <WaitingRequests accounts={accounts} waiting={signIns.waiting} outcome={outcome} say={setOutcome} after={after} />
      <div className="admin-subsection">
        <h3>Add a member</h3>
        <PersonForm searchBase="/api/people" taken={{ entered: [], membersTaken: true }} regions={regions} countries={countries} submitLabel="Add" onSubmit={add} refresh={members} />
        <Notice outcome={shownAt(outcome, 'add member')} onClose={close} />
      </div>
      <div className="admin-subsection">
        <h3>Every member</h3>
        <Notice outcome={shownAt(outcome, 'members')} onClose={close} />
        {members.kind === 'loading' && <p className="muted">Loading…</p>}
        {members.kind === 'error' && <LoadNotice message={members.message} />}
        {members.kind === 'ready' && members.data.length === 0 && <p className="muted">No members yet.</p>}
        {members.kind === 'ready' && members.data.length > 0 && (
          <fieldset className="check-kinds">
            <legend>Show</legend>
            {MEMBER_STATUSES.map(({ status, label }) => (
              <label key={status}>
                <input type="checkbox" checked={statuses.has(status)} onChange={() => toggle(status)} /> {label}
              </label>
            ))}
          </fieldset>
        )}
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
                  <th scope="col">Signed in as</th>
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
                {shown.map((m) => (
                  <Fragment key={m.id}>
                    {editing?.id === m.id ? (
                      <tr>
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
                          <SignedInAs login={signIns.byMember.get(m.id)} />
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
                      <tr className={m.banned ? 'row-error' : ''}>
                        <th scope="row">
                          {m.name}
                          {m.player !== '' && m.player !== m.name && <span className="row-detail muted">{m.player} in the ratings</span>}
                          {m.player === '' && <span className="row-detail muted">not in the ratings</span>}
                          {m.banned && <span className="row-detail">banned: {m.ban_reason}</span>}
                        </th>
                        <td>{m.cge_user ?? ''}</td>
                        <td>{m.discord_name}</td>
                        <td>
                          <SignedInAs login={signIns.byMember.get(m.id)} />
                        </td>
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
                          {signIns.byMember.has(m.id) && (
                            <button type="button" className="btn-antique" onClick={() => void unlink(m)}>
                              Unlink
                            </button>
                          )}
                          {role === 'admin' && !m.banned && (
                            <button type="button" className="btn-antique" onClick={() => void ban(m)}>
                              Ban
                            </button>
                          )}
                          {role === 'admin' && m.banned && (
                            <button type="button" className="btn-antique" onClick={() => void run(req.unbanMember(m.id), `${m.name} is no longer banned.`, `member ${m.id}`)}>
                              Unban
                            </button>
                          )}
                          {role === 'admin' && (
                            <button type="button" className="btn-antique" onClick={() => void remove(m)}>
                              Remove
                            </button>
                          )}
                        </td>
                      </tr>
                    )}
                    <NoticeRow outcome={shownAt(outcome, `member ${m.id}`)} columns={9} onClose={close} />
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      {(signIns.unlinked.length > 0 || shownAt(outcome, 'unlinked') !== null) && (
        <UnlinkedSignIns unlinked={signIns.unlinked} role={role} outcome={outcome} say={setOutcome} after={after} />
      )}
    </section>
  );
}
