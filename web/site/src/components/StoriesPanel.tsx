/** DANeo's Dynastic Milestones & Champion Chronicles panel, for any tournament: its stories, as admin writes them. */
import { playerHref } from '../lib/player-view';

/** A story: a player, the badge beside them (may be empty), a title and the story. */
export type StoryView = { player: string; badge: string; title: string; text: string };

/** StoriesPanel shows the story cards, and nothing for a tournament with none, as DANeo's page does. */
export default function StoriesPanel({ stories }: { stories: readonly StoryView[] }) {
  if (stories.length === 0) return null;
  return (
    <div className="panel-card panel-stories">
      <h2 className="panel-header">
        <span>⚡</span> Master+ Dynastic Milestones &amp; Champion Chronicles
      </h2>
      <div className="stories-grid">
        {stories.map((s, i) => (
          <div key={`${i} ${s.player}`} className="story-card">
            <div className="story-header">
              <a href={playerHref(s.player)} className="story-author">
                <span>👤</span> {s.player}
              </a>
              {s.badge !== '' && <div className="story-badge">{s.badge}</div>}
            </div>
            <div className="story-title">{s.title}</div>
            <div className="story-text">{s.text}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
