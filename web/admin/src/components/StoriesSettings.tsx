/** A tournament's stories, edited and saved together. */
import { addedStory, canAddStory, editedStory, movedStory, removedStory, storiesChanged } from '../lib/admin-edits';
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import SaveBar from './SaveBar';
import { useSave } from './useSave';
import AdminCard from './AdminCard';
import PeoplePick from './PeoplePick';
import { playerChoices } from '../lib/person-form';
import FieldGroup from './FieldGroup';

type Story = AdminTournament['stories'][number];

/** StoriesSettings edits the tournament's stories, as DANeo's pages tell them, and saves them all at once in this order. */
export default function StoriesSettings({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [list, setList] = useState<Story[]>(t.stories);
  const saving = useSave();
  const changed = storiesChanged(t.stories, list);
  const set = (i: number, field: keyof Story, value: string) => setList(editedStory(list, i, field, value));
  const move = (i: number, by: number) => setList(movedStory(list, i, by));
  const save = async () => {
    const r = await saving.run(() => req.send(req.setStories(t.slug, list)), { done: 'Saved. The tournament page shows them at once.' });
    if (r?.ok) await onReload();
  };
  return (
    <AdminCard id="stories-heading" title="Stories">
      <p className="muted">
        The story cards on the tournament page: a player, an optional badge beside them, a title and the story. Up to 12,
        shown in this order. With none, the page shows no stories section.
      </p>
      {list.map((st, i) => (
        <FieldGroup key={i} title={`Story ${i + 1}`}>
          <div className="card-fields">
            <PeoplePick label="Player" value={st.player} onChange={(text) => set(i, 'player', text)} choices={playerChoices} placeholder="" />
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
        </FieldGroup>
      ))}
      <SaveBar save={saving}>
        <button type="button" className="btn-antique" onClick={() => setList(addedStory(list))} disabled={!canAddStory(list)}>
          Add a story
        </button>
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!changed}>
          Save stories
        </button>
      </SaveBar>
    </AdminCard>
  );
}
