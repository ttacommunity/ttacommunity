/** Community leaderboard editions management, for admins. */
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import * as req from '../lib/admin-requests';
import { newEdition, type EditionFieldValues } from '../lib/community-editions';
import { loadFailure, outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import Notice from './Notice';
import AdminCard from './AdminCard';
import EditionFields from './EditionFields';

type CommunityEditionItem = components['schemas']['CommunityEditionItem'];

type EditFormState = EditionFieldValues & { visible: boolean };

/** EditionRow renders an edition's fields with inline editing and save button, and what became of the last change to it. */
function EditionRow({
  edition,
  onReload,
  outcome,
  say,
}: {
  edition: CommunityEditionItem;
  onReload: () => Promise<void>;
  outcome: Outcome | null;
  say: (o: Outcome | null) => void;
}) {
  const at = `edition ${edition.slug}`;
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
    say(outcomeOf(r, `Saved edition ${edition.slug}.`, at));
    if (r.ok) {
      setEditing(false);
      await onReload();
    }
  };

  const toggleVisible = async () => {
    const next = !edition.visible;
    const r = await req.send(req.setCommunityEdition(edition.slug, { visible: next }));
    say(outcomeOf(r, `Edition ${edition.slug} is now ${next ? 'visible' : 'hidden'}.`, at));
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
          <EditionFields value={form} onChange={setForm} />
          <button type="submit" className="btn-antique">
            Save edition
          </button>
        </form>
      )}
      <Notice outcome={shownAt(outcome, at)} onClose={() => say(null)} />
    </div>
  );
}

/** CommunityPanel allows listing, editing and creating community leaderboard editions. */
export default function CommunityPanel() {
  const [list, setList] = useState<CommunityEditionItem[] | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [creating, setCreating] = useState(false);
  const [newForm, setNewForm] = useState<CommunityEditionItem>(newEdition([], new Date().getFullYear()));

  const load = async () => {
    const r = await req.send<CommunityEditionItem[]>(req.communityEditions);
    if (r.ok) {
      setList(r.data);
    } else {
      setOutcome(loadFailure(r.message, 'editions'));
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.createCommunityEdition(newForm));
    setOutcome(outcomeOf(r, `Created edition ${newForm.slug}.`, 'new edition'));
    if (r.ok) {
      setCreating(false);
      setNewForm(newEdition([], new Date().getFullYear()));
      await load();
    }
  };

  return (
    <AdminCard id="community-heading" title="Community leaderboard editions">
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
          <EditionFields value={newForm} onChange={setNewForm} />
          <button type="submit" className="btn-antique" disabled={!newForm.slug || !newForm.name}>
            Create edition
          </button>
        </form>
      )}

      <Notice outcome={shownAt(outcome, 'new edition')} onClose={() => setOutcome(null)} />
      <Notice outcome={shownAt(outcome, 'editions')} />

      {list === null ? (
        <p className="muted">Loading editions…</p>
      ) : list.length === 0 ? (
        <p className="muted">No editions configured yet.</p>
      ) : (
        list.map((e) => (
          <EditionRow key={e.slug} edition={e} onReload={load} outcome={outcome} say={setOutcome} />
        ))
      )}
    </AdminCard>
  );
}
