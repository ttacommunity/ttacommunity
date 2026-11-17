/** The sheet that adds a Reddit feed (its subreddit alone) or edits one: the kinds of post it sends. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import * as req from '../lib/admin-requests';
import { KINDS, kindsChanged, toggledKind } from '../lib/reddit-feeds';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Feed = components['schemas']['RedditFeed'];

interface Props {
  /** The feed to edit, or null to add one. */
  feed: Feed | null;
  opener: HTMLElement;
  /** Runs once the feed is saved, with what to say about it. */
  onSaved: (message: string) => void;
  onClose: () => void;
}

export default function FeedSheet({ feed, opener, onSaved, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [typed, setTyped] = useState('');
  const [kinds, setKinds] = useState<Feed['kinds']>(feed?.kinds ?? []);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const id = feed === null ? 'feed-new' : `feed-${feed.id}`;
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (feed === null) {
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
    <Sheet dialog={dialog} id={id} title={feed === null ? 'Add a Reddit feed' : `Edit r/${feed.subreddit}`} opener={opener} onClose={onClose}>
      <form className="sheet-body" onSubmit={(e) => void save(e)}>
        {feed === null ? (
          <div className="admin-add">
            <label>
              Subreddit
              <input className="input-antique" value={typed} placeholder="throughtheages" onChange={(e) => setTyped(e.target.value)} required />
            </label>
          </div>
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
