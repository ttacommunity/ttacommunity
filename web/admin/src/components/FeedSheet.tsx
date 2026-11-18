/** The sheet that adds a feed, a subreddit or a Twitch streamer by name or an RSS feed by address, or edits a Reddit feed: the kinds of post it sends. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import Segmented from '@ttac/shared/Segmented';
import * as req from '../lib/admin-requests';
import { KINDS, kindsChanged, toggledKind } from '../lib/reddit-feeds';
import { KIND_LABELS, type Kind } from '../lib/feeds';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Feed = components['schemas']['RedditFeed'];
type TwitchFeed = components['schemas']['TwitchFeed'];
type RSSFeed = components['schemas']['RSSFeed'];

/** ADD_FIELDS are what each kind of feed is added by: the field's label and an example. */
const ADD_FIELDS: Record<Kind, { label: string; example: string }> = {
  reddit: { label: 'Subreddit', example: 'throughtheages' },
  twitch: { label: 'Twitch name or channel address', example: 'some_streamer' },
  rss: { label: 'Feed address (https)', example: 'https://news.example.com/feed.xml' },
};

interface Props {
  /** The Reddit feed to edit, or null to add a feed of either kind. */
  feed: Feed | null;
  opener: HTMLElement;
  /** Runs once the feed is saved, with what to say about it. */
  onSaved: (message: string) => void;
  onClose: () => void;
}

export default function FeedSheet({ feed, opener, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [kind, setKind] = useState<Kind>('reddit');
  const [typed, setTyped] = useState('');
  const [kinds, setKinds] = useState<Feed['kinds']>(feed?.kinds ?? []);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = feed === null ? 'feed-new' : `feed-${feed.id}`;
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (feed === null && kind === 'twitch') {
      const r = await req.send<TwitchFeed>(req.addTwitchFeed(typed));
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Added ${r.data.login}. Twitch checks the callback first; choose which endpoints take them above.`);
    } else if (feed === null && kind === 'rss') {
      const r = await req.send<RSSFeed>(req.addRSSFeed(typed));
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Added ${r.data.url}. Its first read, within 10 minutes, posts nothing; choose which endpoints take it above.`);
    } else if (feed === null) {
      const r = await req.send<Feed>(req.addFeed(typed));
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Added r/${r.data.subreddit}. Its first read, within 10 minutes, posts nothing; choose which endpoints take it above.`);
    } else {
      const r = await req.send(req.setFeed(feed.id, { kinds, paused: feed.paused }));
      if (!r.ok) return setOutcome(refused(r.message, id));
      onSaved(`Saved r/${feed.subreddit}.`);
    }
    dialog.current?.close();
  };
  const ready = feed === null ? typed.trim() !== '' : kindsChanged(feed.kinds, kinds);
  return (
    <Sheet dialog={dialog} id={id} title={feed === null ? 'Add a feed' : `Edit r/${feed.subreddit}`} opener={opener} onClose={onClose}>
      <form className="sheet-body" onSubmit={(e) => void save(e)}>
        {feed === null ? (
          <>
            <Segmented<Kind> label="Kind of feed" options={(['reddit', 'twitch', 'rss'] as const).map((k) => ({ value: k, label: KIND_LABELS[k] }))} value={kind} onChange={setKind} />
            <div className="admin-add">
              <label>
                {ADD_FIELDS[kind].label}
                <input className="input-antique" type={kind === 'rss' ? 'url' : 'text'} value={typed} placeholder={ADD_FIELDS[kind].example}
                  onChange={(e) => setTyped(e.target.value)} required />
              </label>
            </div>
          </>
        ) : (
          <>
            <fieldset className="check-kinds">
              <legend>Sends these kinds of post</legend>
              {KINDS.map((k) => (
                <label key={k.key}>
                  <input type="checkbox" checked={kinds.includes(k.key)} onChange={(e) => setKinds(toggledKind(kinds, k.key, e.target.checked))} />{' '}
                  {k.label}
                </label>
              ))}
            </fieldset>
          </>
        )}
        <Notice outcome={outcome} onClose={() => setOutcome(null)} />
        <div className="site-dialog-buttons">
          <button type="button" className="btn-antique" onClick={() => dialog.current?.close()}>
            Cancel
          </button>
          <button type="submit" className="btn-antique" disabled={!ready}>
            {feed === null ? 'Add' : 'Save'}
          </button>
        </div>
      </form>
    </Sheet>
  );
}
