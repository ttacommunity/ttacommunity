/** Titles set by hand, for admins: each replaces the engine's derived title from the next rating run. */
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import { adminRequest } from '../lib/admin-api';

type TitleOverride = components['schemas']['TitleOverride'];
type Title = TitleOverride['title'];

const TITLES: { value: Title; label: string }[] = [
  { value: 'SGM', label: 'Super Grandmaster (SGM)' },
  { value: 'GM', label: 'Grandmaster (GM)' },
  { value: 'M', label: 'Master (M)' },
  { value: 'P', label: 'Platinum (P)' },
  { value: 'G', label: 'Gold (G)' },
  { value: 'S', label: 'Silver (S)' },
  { value: 'B', label: 'Bronze (B)' },
  { value: 'W', label: 'Wood (W)' },
  { value: '', label: 'No title' },
];

function titleLabel(t: Title): string {
  return TITLES.find((x) => x.value === t)?.label ?? t;
}

/** TitlesPanel lists the titles set in admin, and sets or removes one. */
export default function TitlesPanel() {
  const [list, setList] = useState<TitleOverride[] | null>(null);
  const [message, setMessage] = useState('');
  const [form, setForm] = useState({ name: '', title: 'GM' as Title, reason: '' });

  const load = async () => {
    const r = await adminRequest<TitleOverride[]>('/api/titles', {});
    if (r.ok) setList(r.data);
    else setMessage(r.message);
  };
  useEffect(() => {
    void load();
  }, []);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    const name = form.name.trim();
    const r = await adminRequest(`/api/titles/${encodeURIComponent(name)}`, {
      method: 'PUT',
      json: { title: form.title, reason: form.reason.trim() },
    });
    setMessage(r.ok ? `Saved ${name}: ${titleLabel(form.title)}. It shows from the next rating run.` : r.message);
    if (r.ok) setForm({ name: '', title: 'GM', reason: '' });
    await load();
  };
  const remove = async (o: TitleOverride) => {
    if (!window.confirm(`Remove the title set for ${o.name}? The next run derives it again.`)) return;
    const r = await adminRequest(`/api/titles/${encodeURIComponent(o.name)}`, { method: 'DELETE' });
    setMessage(r.ok ? `Removed ${o.name}'s title. The next run derives it again.` : r.message);
    await load();
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="titles-heading">
      <h2 id="titles-heading" className="rating-card-heading">
        Player titles
      </h2>
      <p className="muted">
        A title set here replaces the one the engine derives, from the next rating run. A player still needs a tournament
        game in the last 12 months to show a badge. World Champion comes from the World Championship's podium.
      </p>
      {message !== '' && (
        <p className="admin-message" role="status">
          {message}
        </p>
      )}
      {list !== null && list.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Titles set in admin</caption>
            <thead>
              <tr>
                <th scope="col">Player</th>
                <th scope="col">Title</th>
                <th scope="col">Reason</th>
                <th scope="col">
                  <span className="visually-hidden">Remove</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {list.map((o) => (
                <tr key={o.name}>
                  <th scope="row">{o.name}</th>
                  <td>{titleLabel(o.title)}</td>
                  <td>{o.reason}</td>
                  <td>
                    <button type="button" className="btn-antique" onClick={() => void remove(o)}>
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {list !== null && list.length === 0 && <p className="muted">No titles are set by hand.</p>}
      <form className="admin-add" onSubmit={(e) => void save(e)}>
        <h3>Set a title</h3>
        <label>
          Player (as the leaderboard spells it)
          <input className="input-antique" value={form.name} maxLength={200} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label>
          Title
          <select className="input-antique" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value as Title })}>
            {TITLES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </label>
        <label className="title-reason">
          Reason (shown when hovering the badge)
          <input className="input-antique" value={form.reason} maxLength={300} onChange={(e) => setForm({ ...form, reason: e.target.value })} required />
        </label>
        <button type="submit" className="btn-antique">
          Set title
        </button>
      </form>
    </section>
  );
}
