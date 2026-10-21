/** The admin page: the site's header with admin's tabs (tournaments, blitz, members and their sign-ins, data and rating runs, titles and badges, admins). */
import { Fragment, useEffect, useState, type ReactNode } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { adminRequest, type AdminAccount, type AdminBlitzEditionRow, type AdminMe, type AdminTournamentRow, type Load } from '../lib/admin-api';
import { panelsFor, tabCounts, tabFrom, tabsFor, type Panel, type TabKey } from '../lib/admin-tabs';
import { signInGroups } from '../lib/sign-ins';
import { listed } from '../lib/tournament-groups';
import { settingsHref } from '../lib/tournament-page';
import AdminHeader from './AdminHeader';
import PageError from './PageError';
import { LoadNotice } from './Notice';
import NewGroupForm from './NewGroupForm';
import TournamentTable from './TournamentTable';
import AdminList from './AdminList';
import RunsPanel from './RunsPanel';
import DataCard from './DataCard';
import StandingsCheckPanel from './StandingsCheckPanel';
import TitlesPanel from './TitlesPanel';
import BadgesPanel from './BadgesPanel';
import CommunityPanel from './CommunityPanel';
import BlitzPanel from './BlitzPanel';
import MembersPanel from './MembersPanel';
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
      {tournaments.kind === 'error' && <LoadNotice message={tournaments.message} />}
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

/** SignIns are players' sign-ins, loaded once for the Members tab and the count beside it. */
type SignIns = { accounts: Load<AdminAccount[]>; reload: () => Promise<void> };

/** panel draws one of admin's panels; myName and role are who is signed in, for the admin and members lists. */
function panel(p: Panel, myName: string, role: AdminMe['role'], signIns: SignIns): ReactNode {
  switch (p) {
    case 'tournaments':
      return <Tournaments />;
    case 'community':
      return <CommunityPanel />;
    case 'blitz':
      return <BlitzPanel />;
    case 'members':
      return <MembersPanel role={role} accounts={signIns.accounts} reloadAccounts={signIns.reload} />;
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
  const [accounts, setAccounts] = useState<Load<AdminAccount[]>>({ kind: 'loading' });
  const reloadAccounts = async () => {
    const r = await adminRequest<AdminAccount[]>('/api/accounts', {});
    setAccounts(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  const waiting = accounts.kind === 'ready' ? signInGroups(accounts.data).waiting.length : 0;
  // flagged counts the players' reports waiting for an organiser, across every blitz edition, for the Blitz tab.
  const [flagged, setFlagged] = useState(0);
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) => {
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
      if (!r.ok) return;
      void reloadAccounts();
      void adminRequest<AdminBlitzEditionRow[]>('/api/blitz', {}).then((b) => b.ok && setFlagged(b.data.reduce((n, e) => n + e.flagged, 0)));
      const follow = () => setTab(tabFrom(window.location.hash, r.data.role));
      follow();
      window.addEventListener('hashchange', follow);
    });
  }, []);

  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current={me.kind === 'ready' ? tab : null} counts={tabCounts(waiting, flagged)} />
      <main className="admin-main">
        {me.kind === 'loading' && <p className="muted">Loading…</p>}
        {me.kind === 'error' && <PageError message={me.message} />}
        {me.kind === 'ready' && (
          <>
            <p className="signed-in">
              Signed in as <strong>{me.data.name}</strong> ({me.data.role})
            </p>
            {panelsFor(tab, me.data.role).map((p) => (
              <Fragment key={p}>{panel(p, me.data.name, me.data.role, { accounts, reload: reloadAccounts })}</Fragment>
            ))}
          </>
        )}
      </main>
    </>
  );
}
