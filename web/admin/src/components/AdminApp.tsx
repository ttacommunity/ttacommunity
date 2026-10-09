/** The admin page: who is signed in, rating runs and the admin list for admins, and the top-level tournaments and groups with links to their settings. */
import { useEffect, useState } from 'react';
import { adminRequest, type AdminMe, type AdminTournament } from '../lib/admin-api';
import { topLevel } from '../lib/tournament-groups';
import { settingsHref } from '../lib/tournament-page';
import NewGroupForm from './NewGroupForm';
import TournamentTable from './TournamentTable';
import AdminList from './AdminList';
import RunsPanel from './RunsPanel';
import DataCard from './DataCard';
import StandingsCheckPanel from './StandingsCheckPanel';
import TitlesPanel from './TitlesPanel';
import BadgesPanel from './BadgesPanel';
import './AdminApp.css';

type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** AdminApp loads who is signed in, then the tournaments they can edit. */
export default function AdminApp() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [tournaments, setTournaments] = useState<Load<AdminTournament[]>>({ kind: 'loading' });
  const [making, setMaking] = useState(false);

  const loadTournaments = async () => {
    const r = await adminRequest<AdminTournament[]>('/api/tournaments', {});
    setTournaments(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void loadTournaments();
  }, []);

  if (me.kind === 'loading') return <p className="muted">Loading…</p>;
  if (me.kind === 'error') return <p className="row-error" role="alert">{me.message}</p>;

  return (
    <>
      <p className="signed-in">
        Signed in as <strong>{me.data.name}</strong> ({me.data.role})
      </p>
      <DataCard />
      <StandingsCheckPanel />
      {me.data.role === 'admin' && <RunsPanel />}
      {me.data.role === 'admin' && <AdminList myName={me.data.name} />}
      {me.data.role === 'admin' && <TitlesPanel />}
      {me.data.role === 'admin' && <BadgesPanel />}
      <section className="rating-card admin-card" aria-labelledby="tournaments-heading">
        <h2 id="tournaments-heading" className="rating-card-heading">
          Tournaments
        </h2>
        <p className="muted">
          Each group of related tournaments once, and each tournament related to nothing. A group's settings page lists its
          tournaments; each one's settings, format and regulations, season notes and podiums are on its own page.
        </p>
        {tournaments.kind === 'ready' && (
          <p>
            <button type="button" className="btn-antique" onClick={() => setMaking(!making)} aria-expanded={making}>
              New group
            </button>
          </p>
        )}
        {making && <NewGroupForm />}
        {tournaments.kind === 'loading' && <p className="muted">Loading tournaments…</p>}
        {tournaments.kind === 'error' && <p className="row-error" role="alert">{tournaments.message}</p>}
        {tournaments.kind === 'ready' && (
          <TournamentTable
            rows={topLevel(tournaments.data)}
            all={tournaments.data}
            caption="Each top-level tournament or group and its settings"
            action={(t) => (
              <a className="btn-antique btn-settings" href={settingsHref(t.slug, t.is_group)} aria-label={`Settings for ${t.name}`}>
                Settings
              </a>
            )}
          />
        )}
      </section>
    </>
  );
}
