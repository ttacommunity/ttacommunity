/** Badges, for admins: make one, give it to players and take it back; each shows beside a player's title at once. */
import { badgeChanged } from '../lib/admin-edits';
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import * as req from '../lib/admin-requests';
import { adminRequest } from '../lib/admin-api';

type AdminBadge = components['schemas']['AdminBadge'];

/** BadgeRow is one badge: its label and description to edit, its holders, and a box to give it to someone. */
function BadgeRow({ badge, onReload, say }: { badge: AdminBadge; onReload: () => Promise<void>; say: (m: string) => void }) {
  const [label, setLabel] = useState(badge.label);
  const [description, setDescription] = useState(badge.description);
  const [player, setPlayer] = useState('');
  const save = async () => {
    const r = await req.send(req.setBadge(badge.id, label, description));
    say(r.ok ? `Saved ${label.trim()}.` : r.message);
    await onReload();
  };
  const remove = async () => {
    if (!window.confirm(`Remove the ${badge.label} badge from everyone who holds it?`)) return;
    const r = await req.send(req.removeBadge(badge.id));
    say(r.ok ? `Removed the ${badge.label} badge.` : r.message);
    await onReload();
  };
  const give = async (e: FormEvent) => {
    e.preventDefault();
    const name = player.trim();
    const r = await req.send(req.giveBadge(badge, name));
    say(r.ok ? `Gave ${name} the ${badge.label} badge.` : r.message);
    if (r.ok) setPlayer('');
    await onReload();
  };
  const take = async (name: string) => {
    const r = await req.send(req.takeBadge(badge, name));
    say(r.ok ? `Took the ${badge.label} badge from ${name}.` : r.message);
    await onReload();
  };
  const id = `badge-${badge.id}`;
  return (
    <div className="badge-row">
      <div className="admin-add">
        <label>
          Label
          <input className="input-antique" id={`${id}-label`} value={label} maxLength={16} onChange={(e) => setLabel(e.target.value)} />
        </label>
        <label className="title-reason">
          Description (shown on hover)
          <input className="input-antique" id={`${id}-description`} value={description} maxLength={300} onChange={(e) => setDescription(e.target.value)} />
        </label>
        <button
          type="button"
          className="btn-antique"
          onClick={() => void save()}
          disabled={!badgeChanged(badge, label, description)}
        >
          Save
        </button>
        <button type="button" className="btn-antique" onClick={() => void remove()}>
          Remove badge
        </button>
      </div>
      <p className="badge-holders">
        {badge.players.length === 0 ? (
          <span className="muted">Nobody holds it yet.</span>
        ) : (
          badge.players.map((p) => (
            <span key={p} className="badge-holder">
              {p}
              <button type="button" className="badge-take" onClick={() => void take(p)} aria-label={`Take ${badge.label} from ${p}`}>
                ×
              </button>
            </span>
          ))
        )}
      </p>
      <form className="admin-add" onSubmit={(e) => void give(e)}>
        <label>
          Give it to (as the leaderboard spells the name)
          <input className="input-antique" id={`${id}-player`} value={player} maxLength={200} onChange={(e) => setPlayer(e.target.value)} />
        </label>
        <button type="submit" className="btn-antique" disabled={player.trim() === ''}>
          Give
        </button>
      </form>
    </div>
  );
}

/** BadgesPanel lists the badges and makes new ones. */
export default function BadgesPanel() {
  const [list, setList] = useState<AdminBadge[] | null>(null);
  const [message, setMessage] = useState('');
  const [form, setForm] = useState({ label: '', description: '' });

  const load = async () => {
    const r = await adminRequest<AdminBadge[]>('/api/badges', {});
    if (r.ok) setList(r.data);
    else setMessage(r.message);
  };
  useEffect(() => {
    void load();
  }, []);

  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send<AdminBadge>(req.addBadge(form.label, form.description));
    setMessage(r.ok ? `Made the ${r.data.label} badge. Give it to players below.` : r.message);
    if (r.ok) setForm({ label: '', description: '' });
    await load();
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="badges-heading">
      <h2 id="badges-heading" className="rating-card-heading">
        Badges
      </h2>
      <p className="muted">
        Badges the engine cannot know, such as Organiser. A badge shows beside the player's title on the leaderboard and their
        page within five minutes, with no rating run.
      </p>
      {message !== '' && (
        <p className="admin-message" role="status">
          {message}
        </p>
      )}
      {list?.map((b) => <BadgeRow key={`${b.id}-${b.label}-${b.description}`} badge={b} onReload={load} say={setMessage} />)}
      {list !== null && list.length === 0 && <p className="muted">No badges yet.</p>}
      <form className="admin-add" onSubmit={(e) => void add(e)}>
        <h3>Make a badge</h3>
        <label>
          Label (up to 16 characters)
          <input className="input-antique" value={form.label} maxLength={16} onChange={(e) => setForm({ ...form, label: e.target.value })} required />
        </label>
        <label className="title-reason">
          Description (shown on hover)
          <input className="input-antique" value={form.description} maxLength={300} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </label>
        <button type="submit" className="btn-antique">
          Make badge
        </button>
      </form>
    </section>
  );
}
