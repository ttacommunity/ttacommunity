/** A tournament's or group's page: the groups it is in; a group's settings and members, or a tournament's settings, format and regulations, and seasons. */
import { useEffect, useState } from 'react';
import { adminRequest, type AdminMe, type AdminTournament, type AdminTournamentRow, type Load } from '../lib/admin-api';
import { renamedFrom, slugParam, tournamentLoad } from '../lib/tournament-page';
import { tabsFor } from '../lib/admin-tabs';
import AdminHeader from './AdminHeader';
import GroupPage from './GroupPage';
import RelatedCard from './RelatedCard';
import SettingsCard from './SettingsCard';
import CardSettings from './CardSettings';
import RegulationsCard from './RegulationsCard';
import StoriesSettings from './StoriesSettings';
import SeasonsCard from './SeasonsCard';
import './AdminApp.css';

/** TournamentSettings loads who is signed in and the tournament the address names, then its cards. */
export default function TournamentSettings() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [t, setT] = useState<Load<AdminTournament>>({ kind: 'loading' });
  const [all, setAll] = useState<AdminTournamentRow[]>([]);
  const load = async () => {
    const slug = slugParam(new URLSearchParams(window.location.search));
    if (slug === null) {
      setT({ kind: 'error', message: 'This address names no tournament. Go back to the list.' });
      return;
    }
    const [one, rows] = await Promise.all([
      adminRequest<AdminTournament>(`/api/tournaments/${encodeURIComponent(slug)}`, {}),
      adminRequest<AdminTournamentRow[]>('/api/tournaments', {}),
    ]);
    if (rows.ok) setAll(rows.data);
    setT(tournamentLoad(slug, one, rows));
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void load();
  }, []);

  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current="tournaments" />
      <main className="admin-main">
      {me.kind === 'error' && <p className="row-error" role="alert">{me.message}</p>}
      {me.kind === 'ready' && (
        <p className="signed-in">
          Signed in as <strong>{me.data.name}</strong> ({me.data.role})
        </p>
      )}
      {t.kind === 'loading' && <p className="muted">Loading…</p>}
      {t.kind === 'error' && <p className="row-error" role="alert">{t.message}</p>}
      {t.kind === 'ready' && (
        <>
          <section className="rating-card admin-card page-header" aria-labelledby="page-title">
            <RelatedCard t={t.data} all={all} onReload={load} />
            <h2 id="page-title" className="tournament-title">
              {t.data.name}
            </h2>
            {renamedFrom(t.data.name, t.data.original_name) !== null && (
              <p className="muted">Imported as {renamedFrom(t.data.name, t.data.original_name)}</p>
            )}
          </section>
          {t.data.is_group ? (
            <GroupPage g={t.data} all={all} onReload={load} />
          ) : (
            <>
              <SettingsCard t={t.data} onChange={(next) => setT({ kind: 'ready', data: next })} />
              <CardSettings t={t.data} onReload={load} />
              <RegulationsCard t={t.data} onReload={load} />
              <StoriesSettings t={t.data} onReload={load} />
              <SeasonsCard t={t.data} onReload={load} />
            </>
          )}
        </>
      )}
      </main>
    </>
  );
}
