/** The admin page: who is signed in, rating runs and the admin list for admins, and every tournament with a link to its settings. */
import { useEffect, useState } from 'react';
import { adminRequest, type AdminMe, type AdminTournament } from '../lib/admin-api';
import { SETTINGS, settingsHref } from '../lib/tournament-page';
import AdminList from './AdminList';
import RunsPanel from './RunsPanel';
import DataCard from './DataCard';
import StandingsCheckPanel from './StandingsCheckPanel';
import TitlesPanel from './TitlesPanel';
import BadgesPanel from './BadgesPanel';
import './AdminApp.css';

type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** YesNo marks a setting on or off in the read-only list. */
function YesNo({ on, label, name }: { on: boolean; label: string; name: string }) {
  return (
    <span className={on ? 'yes' : 'no'} title={`${label}: ${on ? 'yes' : 'no'}`} aria-label={`${label} for ${name}: ${on ? 'yes' : 'no'}`}>
      {on ? '✓' : '–'}
    </span>
  );
}

/** AdminApp loads who is signed in, then the tournaments they can edit. */
export default function AdminApp() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [tournaments, setTournaments] = useState<Load<AdminTournament[]>>({ kind: 'loading' });

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
        <p className="muted">Each tournament's settings, format and regulations, season notes and podiums are on its settings page.</p>
        {tournaments.kind === 'loading' && <p className="muted">Loading tournaments…</p>}
        {tournaments.kind === 'error' && <p className="row-error" role="alert">{tournaments.message}</p>}
        {tournaments.kind === 'ready' && (
          <div className="table-responsive">
            <table className="leaderboard-table admin-table tournaments-table">
              <caption className="visually-hidden">Every tournament and its settings</caption>
              <thead>
                <tr>
                  <th scope="col">Tournament</th>
                  {SETTINGS.map((s) => (
                    <th scope="col" key={s.key} title={`${s.label}: ${s.hint}`} className="col-setting">
                      {s.short}
                    </th>
                  ))}
                  <th scope="col" className="col-seasons">
                    Seasons
                  </th>
                  <th scope="col">
                    <span className="visually-hidden">Settings</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {tournaments.data.map((t) => (
                  <tr key={t.slug}>
                    <th scope="row">{t.name}</th>
                    {SETTINGS.map((s) => (
                      <td key={s.key} className="col-setting">
                        <YesNo on={t[s.key]} label={s.label} name={t.name} />
                      </td>
                    ))}
                    <td className="col-seasons">{t.seasons.length}</td>
                    <td>
                      <a className="btn-antique btn-settings" href={settingsHref(t.slug)} aria-label={`Settings for ${t.name}`}>
                        Settings
                      </a>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}
