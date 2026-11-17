/** The sheet that adds or edits a notification endpoint: its name, webhook URL and prefix, and what it takes, site events and Reddit feeds in groups. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import * as req from '../lib/admin-requests';
import { EVENTS, TEXTS, endpointChanged, toggled, toggledFeed } from '../lib/notification-endpoints';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Endpoint = components['schemas']['NotificationEndpoint'];
type Feed = components['schemas']['RedditFeed'];
export type Draft = Pick<
  Endpoint,
  'name' | 'url' | 'events' | 'feeds' | 'prefix' | 'paused' | 'forum' | 'username' | 'avatar_url' | 'role_id' | 'colour' | 'text' | 'image' | 'author'
>;

export const EMPTY: Draft = {
  name: '', url: '', events: [], feeds: [], prefix: '', paused: false, forum: false, username: '', avatar_url: '', role_id: '',
  colour: '#ff4500', text: 'full', image: true, author: true,
};

interface Props {
  /** The endpoint to edit, or null to add one. */
  endpoint: Endpoint | null;
  allFeeds: readonly Feed[];
  opener: HTMLElement;
  /** Runs once the endpoint is saved, with what to say about it. */
  onSaved: (message: string) => void;
  onClose: () => void;
}

export default function EndpointSheet({ endpoint, allFeeds, opener, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const saved = endpoint ?? EMPTY;
  const [draft, setDraft] = useState<Draft>(saved);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = endpoint === null ? 'endpoint-new' : `endpoint-${endpoint.id}`;
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(endpoint === null ? req.addEndpoint(draft) : req.setEndpoint(endpoint.id, draft));
    if (!r.ok) {
      setOutcome(refused(r.message, id));
      return;
    }
    onSaved(endpoint === null ? `Added ${draft.name.trim()}. Test it to see a message arrive.` : `Saved ${draft.name.trim()}.`);
    dialog.current?.close();
  };
  return (
    <Sheet dialog={dialog} id={id} title={endpoint === null ? 'Add an endpoint' : `Edit ${endpoint.name}`} opener={opener} onClose={onClose}>
      <form className="sheet-body" onSubmit={(e) => void save(e)}>
        <div className="admin-add">
          <label>
            Name
            <input className="input-antique" value={draft.name} maxLength={100} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required />
          </label>
          <label>
            Prefix (such as [dev])
            <input className="input-antique" value={draft.prefix} maxLength={20} onChange={(e) => setDraft({ ...draft, prefix: e.target.value })} />
          </label>
        </div>
        <div className="admin-add">
          <label className="title-reason">
            Discord webhook URL
            <input className="input-antique" type="url" value={draft.url} placeholder="https://discord.com/api/webhooks/…"
              onChange={(e) => setDraft({ ...draft, url: e.target.value })} required />
          </label>
        </div>
        <fieldset className="check-kinds">
          <legend>Message</legend>
          <label>
            Shown as (the webhook's name if empty)
            <input className="input-antique" value={draft.username} maxLength={80} placeholder="TTA Reddit" onChange={(e) => setDraft({ ...draft, username: e.target.value })} />
          </label>
          <label>
            Picture URL (the webhook's if empty)
            <input className="input-antique" type="url" value={draft.avatar_url} placeholder="https://…" onChange={(e) => setDraft({ ...draft, avatar_url: e.target.value })} />
          </label>
          <label>
            Side bar colour <input type="color" value={draft.colour} onChange={(e) => setDraft({ ...draft, colour: e.target.value })} />
          </label>
          <label>
            Role to ping (its id; none if empty)
            <input className="input-antique" value={draft.role_id} inputMode="numeric" placeholder="Copy Role ID in Discord"
              onChange={(e) => setDraft({ ...draft, role_id: e.target.value })} />
          </label>
        </fieldset>
        <fieldset className="check-kinds">
          <legend>Posting</legend>
          <label>
            <input type="checkbox" checked={draft.forum} onChange={(e) => setDraft({ ...draft, forum: e.target.checked })} /> A forum channel: each message opens a thread
          </label>
          <label>
            <input type="checkbox" checked={draft.paused} onChange={(e) => setDraft({ ...draft, paused: e.target.checked })} /> Paused: send nothing
          </label>
        </fieldset>
        <fieldset className="check-kinds">
          <legend>Site events</legend>
          {EVENTS.map(({ key, label }) => (
            <label key={key}>
              <input type="checkbox" checked={draft.events.includes(key)} onChange={(e) => setDraft({ ...draft, events: toggled(draft.events, key, e.target.checked) })} /> {label}
            </label>
          ))}
        </fieldset>
        <fieldset className="check-kinds">
          <legend>Reddit feeds: each new post</legend>
          {allFeeds.length === 0 && <p className="muted">No Reddit feeds yet: add one under Reddit feeds.</p>}
          {allFeeds.map((f) => (
            <label key={f.id}>
              <input type="checkbox" checked={draft.feeds.includes(f.id)} onChange={(e) => setDraft({ ...draft, feeds: toggledFeed(draft.feeds, f.id, e.target.checked) })} /> r/
              {f.subreddit}
              {f.paused && <span className="muted"> (paused)</span>}
            </label>
          ))}
        </fieldset>
        <fieldset className="check-kinds">
          <legend>Reddit posts show</legend>
          <label>
            Post text{' '}
            <select className="input-antique" value={draft.text} onChange={(e) => setDraft({ ...draft, text: e.target.value as Draft['text'] })}>
              {TEXTS.map((t) => (
                <option key={t.key} value={t.key}>
                  {t.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            <input type="checkbox" checked={draft.image} onChange={(e) => setDraft({ ...draft, image: e.target.checked })} /> Their image
          </label>
          <label>
            <input type="checkbox" checked={draft.author} onChange={(e) => setDraft({ ...draft, author: e.target.checked })} /> Their author
          </label>
        </fieldset>
        <Notice outcome={outcome} onClose={() => setOutcome(null)} />
        <div className="site-dialog-buttons">
          <button type="button" className="btn-antique" onClick={() => dialog.current?.close()}>
            Cancel
          </button>
          <button type="submit" className="btn-antique" disabled={!endpointChanged(saved, draft)}>
            {endpoint === null ? 'Add' : 'Save'}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
