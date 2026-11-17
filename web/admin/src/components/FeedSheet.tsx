/** The sheet that adds a Reddit feed: the subreddit, as its name, r/name or its address. */
import { useRef, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import Sheet from '@ttac/shared/Sheet';
import * as req from '../lib/admin-requests';
import { refused, type Outcome } from '../lib/outcome';
import Notice from './Notice';

type Feed = components['schemas']['RedditFeed'];

interface Props {
  opener: HTMLElement;
  /** Runs once the feed is added, with the feed. */
  onAdded: (feed: Feed) => void;
  onClose: () => void;
}

export default function FeedSheet({ opener, onAdded, onClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [typed, setTyped] = useState('');
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send<Feed>(req.addFeed(typed));
    if (!r.ok) {
      setOutcome(refused(r.message, 'feed-new'));
      return;
    }
    onAdded(r.data);
    dialog.current?.close();
  };
  return (
    <Sheet dialog={dialog} id="feed-new" title="Add a Reddit feed" opener={opener} onClose={onClose}>
      <form className="sheet-body" onSubmit={(e) => void add(e)}>
        <div className="admin-add">
          <label>
            Subreddit
            <input className="input-antique" value={typed} placeholder="throughtheages" onChange={(e) => setTyped(e.target.value)} required />
          </label>
        </div>
        <Notice outcome={outcome} onClose={() => setOutcome(null)} />
        <div className="site-dialog-buttons">
          <button type="button" className="btn-antique" onClick={() => dialog.current?.close()}>
            Cancel
          </button>
          <button type="submit" className="btn-antique" disabled={typed.trim() === ''}>
            Add
          </button>
        </div>
      </form>
    </Sheet>
  );
}
