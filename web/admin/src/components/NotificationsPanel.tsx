/** Notifications, for admins: the Discord webhooks the api posts site events and feeds to, each tested here, and the feeds: subreddits, Twitch streamers and RSS feeds. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import { useDialog } from '@ttac/shared/Dialog';
import { formatInstant, viewerZone } from '@ttac/shared/time';
import * as req from '../lib/admin-requests';
import { adminRequest } from '../lib/admin-api';
import { endpointStatus, subscriptions } from '../lib/notification-endpoints';
import { feedGroups, feedRows, KIND_LABELS, pauseRequest, removeRequest, resumedText, type FeedRow, type Feeds } from '../lib/feeds';
import { loadFailure, outcomeOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import Notice from './Notice';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';
import EndpointSheet from './EndpointSheet';
import FeedSheet from './FeedSheet';

type Endpoint = components['schemas']['NotificationEndpoint'];
type Feed = components['schemas']['RedditFeed'];
type TwitchFeed = components['schemas']['TwitchFeed'];
type RSSFeed = components['schemas']['RSSFeed'];
type NotificationTest = components['schemas']['NotificationTest'];

/** FeedsCard lists every feed, Reddit's, Twitch's and RSS's, to edit (a Reddit feed's kinds of post), pause, resume or remove, and adds new ones. */
function FeedsCard({ feeds, onReload }: { feeds: Feeds & { reddit: readonly Feed[] }; onReload: () => Promise<void> }) {
  const { reddit } = feeds;
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [editing, setEditing] = useState<{ feed: Feed | null; opener: HTMLElement } | null>(null);
  const dialog = useDialog();
  const zone = viewerZone();
  const rows = feedRows(feeds, (iso) => formatInstant(iso, zone));
  const pause = async (row: FeedRow) => {
    const kinds = row.kind === 'reddit' ? (reddit.find((f) => f.id === row.id)?.kinds ?? []) : [];
    const r = await req.send(pauseRequest(row, kinds));
    setOutcome(outcomeOf(r, row.paused ? resumedText(row) : `Paused ${row.name}.`, 'feeds'));
    await onReload();
  };
  const remove = async (row: FeedRow, opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: `Remove ${row.name}?`, text: 'Nothing more of it is posted, and endpoints stop taking it.', action: 'Remove', final: true }, opener);
    if (!sure) return;
    const r = await req.send(removeRequest(row));
    setOutcome(outcomeOf(r, `Removed ${row.name}.`, 'feeds'));
    await onReload();
  };
  return (
    <AdminCard
      id="feeds-heading"
      title="Feeds"
      actions={
        <button type="button" className="btn-antique" onClick={(e) => setEditing({ feed: null, opener: e.currentTarget })}>
          Add feed
        </button>
      }
    >
      {dialog.element}
      <p className="muted">
        Every source the endpoints above can take. Subreddits and RSS or Atom feeds are read every 10 minutes, and a feed's first read, and its first
        after resuming, posts nothing. A Twitch streamer is posted when they go live, through Twitch's push. A feed that fails 3 reads in a row, and reads
        again after, is posted to the endpoints taking feeds failing.
      </p>
      <Notice outcome={shownAt(outcome, 'feeds')} onClose={() => setOutcome(null)} />
      {rows.length === 0 ? (
        <p className="muted">No feeds yet.</p>
      ) : (
        <AdminTable caption="Every feed, with what it sends and how it is doing">
          <thead>
            <tr>
              <th>Kind</th>
              <th>Source</th>
              <th>Sends</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={`${row.kind}-${row.id}`}>
                <td>{KIND_LABELS[row.kind]}</td>
                <td>
                  <a href={row.href} target="_blank" rel="noopener noreferrer">
                    {row.name}
                  </a>
                </td>
                <td>{row.sends}</td>
                <td>{row.status}</td>
                <td>
                  {row.kind === 'reddit' && (
                    <button type="button" className="btn-settings" onClick={(e) => setEditing({ feed: reddit.find((f) => f.id === row.id) ?? null, opener: e.currentTarget })}>
                      Edit
                    </button>
                  )}
                  <button type="button" className="btn-settings" onClick={() => void pause(row)}>
                    {row.paused ? 'Resume' : 'Pause'}
                  </button>
                  <button type="button" className="btn-settings" onClick={(e) => void remove(row, e.currentTarget)}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </AdminTable>
      )}
      {editing !== null && (
        <FeedSheet
          feed={editing.feed}
          opener={editing.opener}
          onSaved={(message) => {
            setOutcome(succeeded(message, 'feeds'));
            void onReload();
          }}
          onClose={() => setEditing(null)}
        />
      )}
    </AdminCard>
  );
}

/** NotificationsPanel lists the endpoints, each to edit, test or remove, and adds new ones; then the feeds they may take. */
export default function NotificationsPanel() {
  const [list, setList] = useState<Endpoint[] | null>(null);
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [streamers, setStreamers] = useState<TwitchFeed[]>([]);
  const [rssFeeds, setRSSFeeds] = useState<RSSFeed[]>([]);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [editing, setEditing] = useState<{ endpoint: Endpoint | null; opener: HTMLElement } | null>(null);
  const dialog = useDialog();
  const zone = viewerZone();
  const when = (iso: string) => formatInstant(iso, zone);
  const all = { reddit: feeds, twitch: streamers, rss: rssFeeds };
  const groups = feedGroups(all);

  const load = async () => {
    const [endpoints, read, twitch, rss] = await Promise.all([
      adminRequest<Endpoint[]>('/api/notification-endpoints', {}),
      adminRequest<Feed[]>('/api/reddit-feeds', {}),
      adminRequest<TwitchFeed[]>('/api/twitch-feeds', {}),
      adminRequest<RSSFeed[]>('/api/rss-feeds', {}),
    ]);
    if (endpoints.ok) setList(endpoints.data);
    else setOutcome(loadFailure(endpoints.message, 'endpoints'));
    if (read.ok) setFeeds(read.data);
    else setOutcome(loadFailure(read.message, 'endpoints'));
    if (twitch.ok) setStreamers(twitch.data);
    else setOutcome(loadFailure(twitch.message, 'endpoints'));
    if (rss.ok) setRSSFeeds(rss.data);
    else setOutcome(loadFailure(rss.message, 'endpoints'));
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
          Discord channels the site posts to: when a new player signs up, a player asks to be linked, one reports, corrects or flags a blitz result, a
          Reddit or RSS feed has a new post, or a Twitch streamer goes live. Make a webhook in the channel's settings (Integrations, Webhooks), add it here, and choose what it takes.
        </p>
        <Notice outcome={shownAt(outcome, 'endpoints')} onClose={() => setOutcome(null)} />
        {list !== null && list.length === 0 && <p className="muted">No endpoints yet: nothing is posted.</p>}
        {list !== null && list.length > 0 && (
          <AdminTable caption="The Discord channels posted to, each with what it takes">
            <thead>
              <tr>
                <th>Channel</th>
                <th>Takes</th>
                <th>Status</th>
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
                  <td>{subscriptions(e.events, e.feeds, groups.flatMap((g) => g.feeds))}</td>
                  <td>{endpointStatus(e, when)}</td>
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
            groups={groups}
            opener={editing.opener}
            onSaved={(message) => {
              setOutcome(succeeded(message, 'endpoints'));
              void load();
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </AdminCard>
      <FeedsCard feeds={all} onReload={load} />
    </>
  );
}
