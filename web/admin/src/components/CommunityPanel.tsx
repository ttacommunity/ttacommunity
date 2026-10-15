/** Community leaderboard editions management, for admins. */
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import * as req from '../lib/admin-requests';
import { adminRequest } from '../lib/admin-api';
import { newEdition } from '../lib/community-editions';

type CommunityEditionItem = components['schemas']['CommunityEditionItem'];

interface EditFormState {
  name: string;
  title: string;
  short_name: string;
  badge: string;
  maintainer: string;
  maintainer_note: string;
  year: number;
  counting_events: number;
  visible: boolean;
  sort_order: number;
}

/** EditionRow renders an edition's fields with inline editing and save button. */
function EditionRow({
  edition,
  onReload,
  say,
}: {
  edition: CommunityEditionItem;
  onReload: () => Promise<void>;
  say: (m: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<EditFormState>({
    name: edition.name,
    title: edition.title,
    short_name: edition.short_name,
    badge: edition.badge,
    maintainer: edition.maintainer,
    maintainer_note: edition.maintainer_note,
    year: edition.year,
    counting_events: edition.counting_events,
    visible: edition.visible,
    sort_order: edition.sort_order,
  });

  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.setCommunityEdition(edition.slug, form));
    say(r.ok ? `Saved edition ${edition.slug}.` : r.message);
    if (r.ok) {
      setEditing(false);
      await onReload();
    }
  };

  const toggleVisible = async () => {
    const next = !edition.visible;
    const r = await req.send(req.setCommunityEdition(edition.slug, { visible: next }));
    say(r.ok ? `Edition ${edition.slug} is now ${next ? 'visible' : 'hidden'}.` : r.message);
    if (r.ok) await onReload();
  };

  return (
    <div className="badge-row">
      <div className="admin-add">
        <div>
          <strong>{edition.name}</strong> <span className="muted">({edition.slug})</span>{' '}
          <span className="badge-holder">{edition.badge || 'No badge'}</span>
        </div>
        <div>
          <span className="muted">Year:</span> {edition.year} | <span className="muted">Best:</span> {edition.counting_events} | <span className="muted">Sort:</span> {edition.sort_order}
        </div>
        <button
          type="button"
          className="btn-antique"
          onClick={() => void toggleVisible()}
        >
          {edition.visible ? 'Hide from site' : 'Show on site'}
        </button>
        <button
          type="button"
          className="btn-antique"
          onClick={() => setEditing(!editing)}
          aria-expanded={editing}
        >
          {editing ? 'Cancel' : 'Edit'}
        </button>
      </div>
      <p className="muted">
        <strong>Title:</strong> {edition.title || '—'} | <strong>Maintainer:</strong> {edition.maintainer} ({edition.maintainer_note})
      </p>
      {editing && (
        <form className="admin-add" onSubmit={(e) => void save(e)}>
          <label>
            Name
            <input
              className="input-antique"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </label>
          <label>
            Short Name
            <input
              className="input-antique"
              value={form.short_name}
              onChange={(e) => setForm({ ...form, short_name: e.target.value })}
            />
          </label>
          <label className="title-reason">
            Title
            <input
              className="input-antique"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
            />
          </label>
          <label>
            Badge
            <input
              className="input-antique"
              value={form.badge}
              onChange={(e) => setForm({ ...form, badge: e.target.value })}
            />
          </label>
          <label>
            Maintainer
            <input
              className="input-antique"
              value={form.maintainer}
              onChange={(e) => setForm({ ...form, maintainer: e.target.value })}
            />
          </label>
          <label className="title-reason">
            Maintainer Note
            <input
              className="input-antique"
              value={form.maintainer_note}
              onChange={(e) => setForm({ ...form, maintainer_note: e.target.value })}
            />
          </label>
          <label>
            Year
            <input
              type="number"
              className="input-antique"
              value={form.year}
              onChange={(e) => setForm({ ...form, year: parseInt(e.target.value, 10) || 0 })}
            />
          </label>
          <label>
            Counting Events
            <input
              type="number"
              className="input-antique"
              value={form.counting_events}
              onChange={(e) => setForm({ ...form, counting_events: parseInt(e.target.value, 10) || 8 })}
            />
          </label>
          <label>
            Sort Order
            <input
              type="number"
              className="input-antique"
              value={form.sort_order}
              onChange={(e) => setForm({ ...form, sort_order: parseInt(e.target.value, 10) || 0 })}
            />
          </label>
          <button type="submit" className="btn-antique">
            Save edition
          </button>
        </form>
      )}
    </div>
  );
}

/** CommunityPanel allows listing, editing and creating community leaderboard editions. */
export default function CommunityPanel() {
  const [list, setList] = useState<CommunityEditionItem[] | null>(null);
  const [message, setMessage] = useState('');
  const [creating, setCreating] = useState(false);
  const [newForm, setNewForm] = useState<CommunityEditionItem>(newEdition([], new Date().getFullYear()));

  const load = async () => {
    const r = await adminRequest<CommunityEditionItem[]>('/api/community/editions', {});
    if (r.ok) {
      setList(r.data);
    } else {
      setMessage(r.message);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.createCommunityEdition(newForm));
    setMessage(r.ok ? `Created edition ${newForm.slug}.` : r.message);
    if (r.ok) {
      setCreating(false);
      setNewForm(newEdition([], new Date().getFullYear()));
      await load();
    }
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="community-heading">
      <h2 id="community-heading" className="rating-card-heading">
        Community leaderboard editions
      </h2>
      <p className="muted">
        Configure editions, change visibility on the public site, edit maintainer notes, or register new editions without changing code.
      </p>

      <div className="format-bar">
        <button
          type="button"
          className="btn-antique"
          onClick={() => {
            if (!creating) setNewForm(newEdition(list ?? [], new Date().getFullYear()));
            setCreating(!creating);
          }}
          aria-expanded={creating}
        >
          {creating ? 'Cancel' : 'New edition'}
        </button>
      </div>

      {creating && (
        <form className="admin-add" onSubmit={(e) => void create(e)}>
          <label>
            Slug (ID in URL, e.g. race26)
            <input
              className="input-antique"
              value={newForm.slug}
              onChange={(e) => setNewForm({ ...newForm, slug: e.target.value.trim() })}
              required
            />
          </label>
          <label>
            Name
            <input
              className="input-antique"
              value={newForm.name}
              onChange={(e) => setNewForm({ ...newForm, name: e.target.value })}
              required
            />
          </label>
          <label>
            Short Name
            <input
              className="input-antique"
              value={newForm.short_name}
              onChange={(e) => setNewForm({ ...newForm, short_name: e.target.value })}
            />
          </label>
          <label className="title-reason">
            Title
            <input
              className="input-antique"
              value={newForm.title}
              onChange={(e) => setNewForm({ ...newForm, title: e.target.value })}
            />
          </label>
          <label>
            Badge
            <input
              className="input-antique"
              value={newForm.badge}
              onChange={(e) => setNewForm({ ...newForm, badge: e.target.value })}
            />
          </label>
          <label>
            Maintainer
            <input
              className="input-antique"
              value={newForm.maintainer}
              onChange={(e) => setNewForm({ ...newForm, maintainer: e.target.value })}
            />
          </label>
          <label className="title-reason">
            Maintainer Note
            <input
              className="input-antique"
              value={newForm.maintainer_note}
              onChange={(e) => setNewForm({ ...newForm, maintainer_note: e.target.value })}
            />
          </label>
          <label>
            Year
            <input
              type="number"
              className="input-antique"
              value={newForm.year}
              onChange={(e) => setNewForm({ ...newForm, year: parseInt(e.target.value, 10) || 0 })}
            />
          </label>
          <label>
            Counting Events
            <input
              type="number"
              className="input-antique"
              value={newForm.counting_events}
              onChange={(e) => setNewForm({ ...newForm, counting_events: parseInt(e.target.value, 10) || 8 })}
            />
          </label>
          <label>
            Sort Order
            <input
              type="number"
              className="input-antique"
              value={newForm.sort_order}
              onChange={(e) => setNewForm({ ...newForm, sort_order: parseInt(e.target.value, 10) || 0 })}
            />
          </label>
          <button type="submit" className="btn-antique" disabled={!newForm.slug || !newForm.name}>
            Create edition
          </button>
        </form>
      )}

      {message !== '' && (
        <p className="row-error" role="alert">
          {message}
        </p>
      )}

      {list === null ? (
        <p className="muted">Loading editions…</p>
      ) : list.length === 0 ? (
        <p className="muted">No editions configured yet.</p>
      ) : (
        list.map((e) => (
          <EditionRow key={e.slug} edition={e} onReload={load} say={setMessage} />
        ))
      )}
    </section>
  );
}
