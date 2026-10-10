/** A tournament's stories, edited and saved together. */
import { addedStory, canAddStory, editedStory, movedStory, removedStory, storiesChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';

type Story = AdminTournament['stories'][number];

/** StoriesSettings edits the tournament's stories, as DANeo's pages tell them, and saves them all at once in this order. */
export default function StoriesSettings({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [list, setList] = useState<Story[]>(t.stories);
  const [status, setStatus] = useState('');
  const changed = storiesChanged(t.stories, list);
  const set = (i: number, field: keyof Story, value: string) => setList(editedStory(list, i, field, value));
  const move = (i: number, by: number) => setList(movedStory(list, i, by));
  const save = async () => {
    setStatus('Saving…');
    const r = await req.send(req.setStories(t.slug, list));
    setStatus(r.ok ? 'Saved. The tournament page shows them at once.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="stories-heading">
      <h2 id="stories-heading" className="rating-card-heading">
        Stories
      </h2>
      <p className="muted">
        The story cards on the tournament page: a player, an optional badge beside them, a title and the story. Up to 12,
        shown in this order. With none, the page shows no stories section.
      </p>
      {list.map((st, i) => (
        <fieldset key={i} className="story-box">
          <legend>Story {i + 1}</legend>
          <div className="card-fields">
            <label>
              Player
              <input className="input-antique" id={`story-${i}-player`} value={st.player} maxLength={200} onChange={(e) => set(i, 'player', e.target.value)} />
            </label>
            <label>
              Badge
              <input className="input-antique"
                id={`story-${i}-badge`}
                value={st.badge}
                maxLength={60}
                placeholder="e.g. Reigning World Champion"
                onChange={(e) => set(i, 'badge', e.target.value)}
              />
            </label>
            <label className="card-tagline">
              Title
              <input className="input-antique" id={`story-${i}-title`} value={st.title} maxLength={120} onChange={(e) => set(i, 'title', e.target.value)} />
            </label>
            <label className="card-summary">
              Story
              <textarea className="input-antique" id={`story-${i}-text`} value={st.text} maxLength={1000} rows={3} onChange={(e) => set(i, 'text', e.target.value)} />
            </label>
          </div>
          <div className="note-actions">
            <button type="button" className="btn-antique" onClick={() => move(i, -1)} disabled={i === 0} aria-label={`Move story ${i + 1} up`}>
              ↑
            </button>
            <button
              type="button"
              className="btn-antique"
              onClick={() => move(i, 1)}
              disabled={i === list.length - 1}
              aria-label={`Move story ${i + 1} down`}
            >
              ↓
            </button>
            <button type="button" className="btn-antique" onClick={() => setList(removedStory(list, i))}>
              Remove
            </button>
          </div>
        </fieldset>
      ))}
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => setList(addedStory(list))} disabled={!canAddStory(list)}>
          Add a story
        </button>
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!changed}>
          Save stories
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </section>
  );
}
