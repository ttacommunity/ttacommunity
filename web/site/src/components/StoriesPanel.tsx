/** DANeo's Dynastic Milestones & Champion Chronicles panel, for any tournament: its stories, as admin writes them. */
import PlayerLink from './PlayerLink';
import Panel from './Panel';
import Highlight from './Highlight';

/** A story: a player, the badge beside them (may be empty), a title and the story. */
export type StoryView = { player: string; badge: string; title: string; text: string };

/** StoriesPanel shows each story as a highlight, and nothing for a tournament with none, as DANeo's page does. */
export default function StoriesPanel({ stories }: { stories: readonly StoryView[] }) {
  if (stories.length === 0) return null;
  return (
    <Panel id="stories" icon="⚡" title="Master+ Dynastic Milestones & Champion Chronicles">
      <div className="stories-grid">
        {stories.map((s, i) => (
          <Highlight key={`${i} ${s.player}`}>
            <div className="story-header">
              <PlayerLink name={s.player} className="story-author">
                <span>👤</span> {s.player}
              </PlayerLink>
              {s.badge !== '' && <div className="tag">{s.badge}</div>}
            </div>
            <div className="story-title">{s.title}</div>
            <div className="story-text">{s.text}</div>
          </Highlight>
        ))}
      </div>
    </Panel>
  );
}
