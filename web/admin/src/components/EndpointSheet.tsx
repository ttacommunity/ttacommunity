/** The sheet that adds or edits a notification endpoint: its name, webhook URL and prefix, and what it takes, site events and feeds in groups. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import * as req from '../lib/admin-requests';
import { EVENTS, TEXTS, endpointChanged, endpointSetting, toggled, toggledFeed } from '../lib/notification-endpoints';
import { optionsFor, withOptions, type FeedChoice, type Kind } from '../lib/feeds';
import FeedOptions from './FeedOptions';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Endpoint = components['schemas']['NotificationEndpoint'];
export type Draft = { url: string } & Pick<
  Endpoint,
  'name' | 'webhook_id' | 'events' | 'feeds' | 'feed_options' | 'prefix' | 'paused' | 'forum' | 'username' | 'avatar_url' | 'role_id' | 'thread_id' | 'colour' | 'text' | 'image'
>;

export const EMPTY: Draft = {
  name: '', url: '', webhook_id: '', events: [], feeds: [], feed_options: [], prefix: '', paused: false, forum: false, username: '', avatar_url: '', role_id: '',
  thread_id: '', colour: '#ff4500', text: 'full', image: true,
};

interface Props {
  /** The endpoint to edit, or null to add one. */
  endpoint: Endpoint | null;
  /** The feeds it may take, by kind. */
  groups: readonly { kind: Kind; label: string; feeds: FeedChoice[] }[];
  opener: HTMLElement;
  /** Runs once the endpoint is saved, with what to say about it. */
  onSaved: (message: string) => void;
  onClose: () => void;
}

export default function EndpointSheet({ endpoint, groups, opener, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const saved: Draft = endpoint === null ? EMPTY : { ...endpoint, url: '' };
  const [draft, setDraft] = useState<Draft>(saved);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = endpoint === null ? 'endpoint-new' : `endpoint-${endpoint.id}`;
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(endpoint === null ? req.addEndpoint(endpointSetting(draft)) : req.setEndpoint(endpoint.id, endpointSetting(draft)));
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
            {saved.webhook_id === '' ? 'Discord webhook URL' : `Discord webhook URL (webhook ${saved.webhook_id} saved; paste a new one to replace it)`}
            <input className="input-antique" type="url" value={draft.url} autoComplete="off"
              placeholder={saved.webhook_id === '' ? 'https://discord.com/api/webhooks/…' : 'Kept as saved'}
              onChange={(e) => setDraft({ ...draft, url: e.target.value })} required={saved.webhook_id === ''} />
          </label>
        </div>
        <fieldset className="check-kinds">
          <legend>Message</legend>
          <label>
            Shown as (the webhook's name if empty)
            <input className="input-antique" value={draft.username} maxLength={80} placeholder="TTA News" onChange={(e) => setDraft({ ...draft, username: e.target.value })} />
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
            <input type="checkbox" checked={draft.forum} onChange={(e) => setDraft({ ...draft, forum: e.target.checked })} /> A forum channel: each message opens a thread, unless a thread is set below
          </label>
          <label>
            Post into an existing thread or forum post (its id; none if empty)
            <input className="input-antique" value={draft.thread_id} inputMode="numeric" placeholder="Copy Thread ID in Discord"
              onChange={(e) => setDraft({ ...draft, thread_id: e.target.value })} />
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
          <legend>Feeds: what this channel takes</legend>
          {groups.length === 0 && <p className="muted">No feeds yet: add one under Feeds.</p>}
          {groups.map((g) => (
            <div key={g.kind} className="feed-group">
              <span className="feed-group-label">{g.label}</span>
              {g.feeds.map((f) => (
                <div key={f.id}>
                  <label>
                    <input type="checkbox" checked={draft.feeds.includes(f.id)} onChange={(e) => setDraft({ ...draft, feeds: toggledFeed(draft.feeds, f.id, e.target.checked) })} />{' '}
                    {f.name}
                    {f.paused && <span className="muted"> (paused)</span>}
                  </label>
                  {draft.feeds.includes(f.id) && (
                    <FeedOptions kind={f.kind} options={optionsFor(draft.feed_options, f.id)}
                      onChange={(change) => setDraft((d) => ({ ...d, feed_options: withOptions(d.feed_options, f.id, change) }))} />
                  )}
                </div>
              ))}
            </div>
          ))}
        </fieldset>
        <fieldset className="check-kinds">
          <legend>Feed items show</legend>
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
