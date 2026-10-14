/** The admin page: the site's header with admin's tabs (tournaments, data and rating runs, titles and badges, admins). */
import { Fragment, useEffect, useState, type ReactNode } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { adminRequest, type AdminMe, type AdminTournamentRow, type Load } from '../lib/admin-api';
import { panelsFor, tabFrom, tabsFor, type Panel, type TabKey } from '../lib/admin-tabs';
import { listed } from '../lib/tournament-groups';
import { settingsHref } from '../lib/tournament-page';
import AdminHeader from './AdminHeader';
import PageError from './PageError';
import NewGroupForm from './NewGroupForm';
import TournamentTable from './TournamentTable';
import AdminList from './AdminList';
import RunsPanel from './RunsPanel';
import DataCard from './DataCard';
import StandingsCheckPanel from './StandingsCheckPanel';
import TitlesPanel from './TitlesPanel';
import BadgesPanel from './BadgesPanel';
import CommunityPanel from './CommunityPanel';
import './AdminApp.css';

/** Tournaments lists the top-level tournaments and groups, rated or unrated, with New group. */
function Tournaments() {
  const [tournaments, setTournaments] = useState<Load<AdminTournamentRow[]>>({ kind: 'loading' });
  const [making, setMaking] = useState(false);
  const [rated, setRated] = useState(true);
  useEffect(() => {
    void adminRequest<AdminTournamentRow[]>('/api/tournaments', {}).then((r) =>
      setTournaments(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
  }, []);
  return (
    <section className="rating-card admin-card" aria-labelledby="tournaments-heading">
      <h2 id="tournaments-heading" className="rating-card-heading">
        Tournaments
      </h2>
      <p className="muted">
        Each group of related tournaments once, and each tournament related to nothing. A name opens what is in it;
        Settings opens its settings.
      </p>
      <div className="format-bar">
        <Segmented
          label="Which tournaments"
          options={[
            { value: 'rated', label: 'Rated' },
            { value: 'unrated', label: 'Unrated' },
          ]}
          value={rated ? 'rated' : 'unrated'}
          onChange={(v) => setRated(v === 'rated')}
        />
        <div className="format-bar-actions">
          <button type="button" className="btn-antique" onClick={() => setMaking(!making)} aria-expanded={making}>
            New group
          </button>
        </div>
      </div>
      {making && <NewGroupForm />}
      {tournaments.kind === 'loading' && <p className="muted">Loading tournaments…</p>}
      {tournaments.kind === 'error' && <p className="row-error" role="alert">{tournaments.message}</p>}
      {tournaments.kind === 'ready' && (
        <TournamentTable
          rows={listed(tournaments.data, rated)}
          all={tournaments.data}
          caption={`Each top-level ${rated ? 'rated' : 'unrated'} tournament or group and its settings`}
          action={(t) => (
            <a className="btn-antique btn-settings" href={settingsHref(t.slug, t.is_group)} aria-label={`Settings for ${t.name}`}>
              Settings
            </a>
          )}
        />
      )}
    </section>
  );
}

/** panel draws one of admin's panels; myName is who is signed in, for the admin list. */
function panel(p: Panel, myName: string): ReactNode {
  switch (p) {
    case 'tournaments':
      return <Tournaments />;
    case 'community':
      return <CommunityPanel />;
    case 'data':
      return <DataCard />;
    case 'runs':
      return <RunsPanel />;
    case 'standings':
      return <StandingsCheckPanel />;
    case 'titles':
      return <TitlesPanel />;
    case 'badges':
      return <BadgesPanel />;
    case 'admins':
      return <AdminList myName={myName} />;
  }
}

/** AdminApp loads who is signed in, then shows the tab the address names, following it as it changes. */
export default function AdminApp() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [tab, setTab] = useState<TabKey>('tournaments');
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) => {
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
      if (!r.ok) return;
      const follow = () => setTab(tabFrom(window.location.hash, r.data.role));
      follow();
      window.addEventListener('hashchange', follow);
    });
  }, []);

  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current={me.kind === 'ready' ? tab : null} />
      <main className="admin-main">
        {me.kind === 'loading' && <p className="muted">Loading…</p>}
        {me.kind === 'error' && <PageError message={me.message} />}
        {me.kind === 'ready' && (
          <>
            <p className="signed-in">
              Signed in as <strong>{me.data.name}</strong> ({me.data.role})
            </p>
            {panelsFor(tab, me.data.role).map((p) => (
              <Fragment key={p}>{panel(p, me.data.name)}</Fragment>
            ))}
          </>
        )}
      </main>
    </>
  );
}
