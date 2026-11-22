/** The sheet that adds a feed: a Twitch streamer by name, an RSS or Atom feed by address, or YouTube Live, once. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import Segmented from '@ttac/shared/Segmented';
import * as req from '../lib/admin-requests';
import { addableKinds, KIND_LABELS, type Kind } from '../lib/feeds';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type TwitchFeed = components['schemas']['TwitchFeed'];
type RSSFeed = components['schemas']['RSSFeed'];
type YouTubeFeed = components['schemas']['YouTubeFeed'];

/** ADD_FIELDS are what a Twitch streamer and an RSS feed are added by: the field's label and an example. YouTube Live takes none. */
const ADD_FIELDS: Record<Exclude<Kind, 'youtube'>, { label: string; example: string }> = {
  rss: { label: 'Feed address (https)', example: 'https://news.example.com/feed.xml' },
  twitch: { label: 'Twitch name or channel address', example: 'some_streamer' },
};

interface Props {
  opener: HTMLElement;
  /** Whether the YouTube Live feed is there already, so it is not offered. */
  hasYouTube: boolean;
  /** Runs once the feed is added, with what to say about it. */
  onSaved: (message: string) => void;
  onClose: () => void;
}

export default function FeedSheet({ opener, hasYouTube, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [kind, setKind] = useState<Kind>('rss');
  const [typed, setTyped] = useState('');
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = 'feed-new';
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (kind === 'youtube') {
      const r = await req.send<YouTubeFeed>(req.addYouTubeFeed());
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Added YouTube Live, watching ${r.data.channels} Movies channels; choose which endpoints take it above.`);
    } else if (kind === 'twitch') {
      const r = await req.send<TwitchFeed>(req.addTwitchFeed(typed));
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Added ${r.data.login}. Twitch checks the callback first; choose which endpoints take them above.`);
    } else {
      const r = await req.send<RSSFeed>(req.addRSSFeed(typed));
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Added ${r.data.url}. Its first read, within 10 minutes, posts nothing; choose which endpoints take it above.`);
    }
    dialog.current?.close();
  };
  return (
    <Sheet
      dialog={dialog}
      id={id}
      title="Add a feed"
      opener={opener}
      onClose={onClose}
      actions={
        <>
          <button type="button" className="btn-quiet" onClick={() => dialog.current?.close()}>
            Cancel
          </button>
          <button type="submit" form={`${id}-form`} className="btn-antique" disabled={kind !== 'youtube' && typed.trim() === ''}>
            Add
          </button>
        </>
      }
    >
      <form id={`${id}-form`} className="sheet-body" onSubmit={(e) => void save(e)}>
        <Segmented<Kind> label="Kind of feed" options={addableKinds(hasYouTube).map((k) => ({ value: k, label: KIND_LABELS[k] }))} value={kind} onChange={setKind} />
        {kind === 'youtube' ? (
          <p className="muted">
            Posts a live stream of any channel on the Movies tab when it starts, if Movies shows its video. YouTube tells of a stream when it is
            scheduled or goes live; the api then checks it every minute near its start.
          </p>
        ) : (
          <div className="admin-add">
            <label>
              {ADD_FIELDS[kind].label}
              <input className="input-antique" type={kind === 'rss' ? 'url' : 'text'} value={typed} placeholder={ADD_FIELDS[kind].example}
                onChange={(e) => setTyped(e.target.value)} required />
            </label>
          </div>
        )}
        <Notice outcome={outcome} onClose={() => setOutcome(null)} />
      </form>
    </Sheet>
  );
}
