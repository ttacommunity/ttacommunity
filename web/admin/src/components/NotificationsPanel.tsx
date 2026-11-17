/** Notifications, for admins: the Discord webhooks the api posts site events and Reddit posts to, each tested here, and the Reddit feeds read. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import { useDialog } from '@ttac/shared/Dialog';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import * as req from '../lib/admin-requests';
import { adminRequest } from '../lib/admin-api';
import { subscriptions } from '../lib/notification-endpoints';
import { feedStatus } from '../lib/reddit-feeds';
import { loadFailure, outcomeOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import Notice from './Notice';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';
import EndpointSheet from './EndpointSheet';
import FeedSheet from './FeedSheet';

type Endpoint = components['schemas']['NotificationEndpoint'];
type Feed = components['schemas']['RedditFeed'];
type NotificationTest = components['schemas']['NotificationTest'];

/** FeedsCard lists the Reddit feeds, to pause, resume or remove, and adds new ones. */
function FeedsCard({ feeds, onReload }: { feeds: readonly Feed[]; onReload: () => Promise<void> }) {
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [adding, setAdding] = useState<HTMLElement | null>(null);
  const dialog = useDialog();
  const zone = viewerZone();
  const when = (iso: string) => formatInstant(iso, zone);
  const pause = async (f: Feed) => {
    const r = await req.send(req.pauseFeed(f.id, !f.paused));
    setOutcome(outcomeOf(r, f.paused ? `Resumed r/${f.subreddit}: posts from its next read on are posted.` : `Paused r/${f.subreddit}.`, 'feeds'));
    await onReload();
  };
  const remove = async (f: Feed, opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: `Remove r/${f.subreddit}?`, text: 'Its new posts are no longer posted, and endpoints stop taking it.', action: 'Remove', final: true }, opener);
    if (!sure) return;
    const r = await req.send(req.removeFeed(f.id));
    setOutcome(outcomeOf(r, `Removed r/${f.subreddit}.`, 'feeds'));
    await onReload();
  };
  return (
    <AdminCard
      id="reddit-feeds-heading"
      title="Reddit feeds"
      actions={
        <button type="button" className="btn-antique" onClick={(e) => setAdding(e.currentTarget)}>
          Add feed
        </button>
      }
    >
      {dialog.element}
      <p className="muted">
        Subreddits read every 10 minutes. Each new post goes to the endpoints that take its feed (choose them above). A feed's first read, and its first
        after resuming, posts nothing, so older posts are not posted.
      </p>
      <Notice outcome={shownAt(outcome, 'feeds')} onClose={() => setOutcome(null)} />
      {feeds.length === 0 ? (
        <p className="muted">No feeds yet.</p>
      ) : (
        <AdminTable caption="The subreddits read, each with how its last read went">
          <thead>
            <tr>
              <th>Subreddit</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {feeds.map((f) => (
              <tr key={f.id}>
                <td>
                  <a href={`https://www.reddit.com/r/${encodeURIComponent(f.subreddit)}/new/`} target="_blank" rel="noopener noreferrer">
                    r/{f.subreddit}
                  </a>
                </td>
                <td>{feedStatus(f, when)}</td>
                <td>
                  <button type="button" className="btn-settings" onClick={() => void pause(f)}>
                    {f.paused ? 'Resume' : 'Pause'}
                  </button>
                  <button type="button" className="btn-settings" onClick={(e) => void remove(f, e.currentTarget)}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </AdminTable>
      )}
      {adding !== null && (
        <FeedSheet
          opener={adding}
          onAdded={(f) => {
            setOutcome(succeeded(`Added r/${f.subreddit}. Its first read, within 10 minutes, posts nothing; choose which endpoints take it above.`, 'feeds'));
            void onReload();
          }}
          onClose={() => setAdding(null)}
        />
      )}
    </AdminCard>
  );
}

/** NotificationsPanel lists the endpoints, each to edit, test or remove, and adds new ones; then the Reddit feeds they may take. */
export default function NotificationsPanel() {
  const [list, setList] = useState<Endpoint[] | null>(null);
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [editing, setEditing] = useState<{ endpoint: Endpoint | null; opener: HTMLElement } | null>(null);
  const dialog = useDialog();

  const load = async () => {
    const [endpoints, read] = await Promise.all([adminRequest<Endpoint[]>('/api/notification-endpoints', {}), adminRequest<Feed[]>('/api/reddit-feeds', {})]);
    if (endpoints.ok) setList(endpoints.data);
    else setOutcome(loadFailure(endpoints.message, 'endpoints'));
    if (read.ok) setFeeds(read.data);
    else setOutcome(loadFailure(read.message, 'endpoints'));
  };
  useEffect(() => {
    void load();
  }, []);

  const test = async (e: Endpoint) => {
    const r = await req.send<NotificationTest>(req.testEndpoint(e.id));
    if (!r.ok) setOutcome(refused(r.message, 'endpoints'));
    else setOutcome(r.data.delivered ? succeeded(`${e.name}: ${r.data.message}`, 'endpoints') : refused(`${e.name}: ${r.data.message}`, 'endpoints'));
  };
  const remove = async (e: Endpoint, opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: `Remove ${e.name}?`, text: 'Nothing more is posted to it.', action: 'Remove', final: true }, opener);
    if (!sure) return;
    const r = await req.send(req.removeEndpoint(e.id));
    setOutcome(outcomeOf(r, `Removed ${e.name}.`, 'endpoints'));
    await load();
  };

  return (
    <>
      <AdminCard
        id="notifications-heading"
        title="Notifications"
        actions={
          <button type="button" className="btn-antique" onClick={(e) => setEditing({ endpoint: null, opener: e.currentTarget })}>
            Add endpoint
          </button>
        }
      >
        {dialog.element}
        <p className="muted">
          Discord channels the site posts to: when a new player signs up, a player asks to be linked, one reports, corrects or flags a blitz result, or a
          Reddit feed has a new post. Make a webhook in the channel's settings (Integrations, Webhooks), add it here, and choose what it takes.
        </p>
        <Notice outcome={shownAt(outcome, 'endpoints')} onClose={() => setOutcome(null)} />
        {list !== null && list.length === 0 && <p className="muted">No endpoints yet: nothing is posted.</p>}
        {list !== null && list.length > 0 && (
          <AdminTable caption="The Discord channels posted to, each with what it takes">
            <thead>
              <tr>
                <th>Channel</th>
                <th>Takes</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {list.map((e) => (
                <tr key={e.id}>
                  <td>
                    {e.name}
                    {e.prefix !== '' && <span className="muted"> {e.prefix}</span>}
                  </td>
                  <td>{subscriptions(e.events, e.feeds, feeds)}</td>
                  <td>
                    <button type="button" className="btn-settings" onClick={(ev) => setEditing({ endpoint: e, opener: ev.currentTarget })}>
                      Edit
                    </button>
                    <button type="button" className="btn-settings" onClick={() => void test(e)}>
                      Test
                    </button>
                    <button type="button" className="btn-settings" onClick={(ev) => void remove(e, ev.currentTarget)}>
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
        )}
        {editing !== null && (
          <EndpointSheet
            endpoint={editing.endpoint}
            allFeeds={feeds}
            opener={editing.opener}
            onSaved={(message) => {
              setOutcome(succeeded(message, 'endpoints'));
              void load();
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </AdminCard>
      <FeedsCard feeds={feeds} onReload={load} />
    </>
  );
}
