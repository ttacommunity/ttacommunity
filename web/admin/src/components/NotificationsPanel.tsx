/** Notifications, for admins: the Discord webhooks the api posts site events and feeds to, each tested here, and the feeds: Twitch streamers, RSS feeds and YouTube Live. */
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
type TwitchFeed = components['schemas']['TwitchFeed'];
type RSSFeed = components['schemas']['RSSFeed'];
type YouTubeFeed = components['schemas']['YouTubeFeed'];
type NotificationTest = components['schemas']['NotificationTest'];

/** FeedsCard lists every feed, Twitch's, RSS's and YouTube Live, to pause, resume or remove, and adds new ones. */
function FeedsCard({ feeds, onReload }: { feeds: Feeds; onReload: () => Promise<void> }) {
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [adding, setAdding] = useState<HTMLElement | null>(null);
  const dialog = useDialog();
  const zone = viewerZone();
  const rows = feedRows(feeds, (iso) => formatInstant(iso, zone));
  const pause = async (row: FeedRow) => {
    const r = await req.send(pauseRequest(row));
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
        <button type="button" className="btn-antique" onClick={(e) => setAdding(e.currentTarget)}>
          Add feed
        </button>
      }
    >
      {dialog.element}
      <p className="muted">
        Every source the endpoints above can take. RSS and Atom feeds are read every 10 minutes, and a feed's first read, and its first
        after resuming, posts nothing. A Twitch streamer is posted when they go live, through Twitch's push. YouTube Live posts the live streams of
        the Movies tab's channels when they start. A feed that fails 3 reads in a row, and reads
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
      {adding !== null && (
        <FeedSheet
          opener={adding}
          hasYouTube={feeds.youtube.length > 0}
          onSaved={(message) => {
            setOutcome(succeeded(message, 'feeds'));
            void onReload();
          }}
          onClose={() => setAdding(null)}
        />
      )}
    </AdminCard>
  );
}

/** NotificationsPanel lists the endpoints, each to edit, test or remove, and adds new ones; then the feeds they may take. */
export default function NotificationsPanel() {
  const [list, setList] = useState<Endpoint[] | null>(null);
  const [streamers, setStreamers] = useState<TwitchFeed[]>([]);
  const [rssFeeds, setRSSFeeds] = useState<RSSFeed[]>([]);
  const [youtubeFeeds, setYouTubeFeeds] = useState<YouTubeFeed[]>([]);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [editing, setEditing] = useState<{ endpoint: Endpoint | null; opener: HTMLElement } | null>(null);
  const dialog = useDialog();
  const zone = viewerZone();
  const when = (iso: string) => formatInstant(iso, zone);
  const all = { twitch: streamers, rss: rssFeeds, youtube: youtubeFeeds };
  const groups = feedGroups(all);

  const load = async () => {
    const [endpoints, twitch, rss, youtube] = await Promise.all([
      adminRequest<Endpoint[]>('/api/notification-endpoints', {}),
      adminRequest<TwitchFeed[]>('/api/twitch-feeds', {}),
      adminRequest<RSSFeed[]>('/api/rss-feeds', {}),
      adminRequest<YouTubeFeed[]>('/api/youtube-feeds', {}),
    ]);
    if (endpoints.ok) setList(endpoints.data);
    else setOutcome(loadFailure(endpoints.message, 'endpoints'));
    if (twitch.ok) setStreamers(twitch.data);
    else setOutcome(loadFailure(twitch.message, 'endpoints'));
    if (rss.ok) setRSSFeeds(rss.data);
    else setOutcome(loadFailure(rss.message, 'endpoints'));
    if (youtube.ok) setYouTubeFeeds(youtube.data);
    else setOutcome(loadFailure(youtube.message, 'endpoints'));
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
          Discord channels the site posts to: when a new player signs up, a player asks to be linked, one reports, corrects or flags a blitz result, new
          ratings are published (and their top 10 changed), a blitz round is published or its deadline nears, an edition finishes, a
          RSS feed has a new item, or a Twitch streamer or a Movies channel on YouTube goes live. Make a webhook in the channel's settings (Integrations, Webhooks), add it here, and choose what it takes. A bot channel is
          where the bot makes each blitz game's private thread: add it by the channel's id.
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
