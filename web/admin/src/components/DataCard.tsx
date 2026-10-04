/** The loaded data at a glance: games, players, and when they were last loaded. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import { adminDownload, adminRequest, exportFileName } from '../lib/admin-api';

type DataSummary = components['schemas']['DataSummary'];

const count = (n: number) => n.toLocaleString();

/** DataCard loads the summary once and shows it, warning when games sit in no tournament. */
export default function DataCard() {
  const [d, setD] = useState<DataSummary | null>(null);
  const [message, setMessage] = useState('');
  const [download, setDownload] = useState('');
  const takeExport = async () => {
    setDownload('Preparing the download…');
    const r = await adminDownload('/api/export');
    if (!r.ok) {
      setDownload(r.message);
      return;
    }
    const url = URL.createObjectURL(r.data);
    const a = document.createElement('a');
    a.href = url;
    a.download = exportFileName(new Date());
    a.click();
    URL.revokeObjectURL(url);
    setDownload('Downloaded. The load job restores it: load -restore <folder> (unzipped).');
  };
  useEffect(() => {
    void adminRequest<DataSummary>('/api/data', {}).then((r) => (r.ok ? setD(r.data) : setMessage(r.message)));
  }, []);

  return (
    <section className="rating-card admin-card" aria-labelledby="data-heading">
      <h2 id="data-heading" className="rating-card-heading">
        Data
      </h2>
      {message !== '' && (
        <p className="row-error" role="alert">
          {message}
        </p>
      )}
      {d === null && message === '' && <p className="muted">Loading…</p>}
      {d !== null && (
        <>
          <dl className="data-facts">
            <div>
              <dt>Games played</dt>
              <dd>{count(d.games)}</dd>
            </div>
            <div>
              <dt>Never played</dt>
              <dd>{count(d.never_played)}</dd>
            </div>
            <div>
              <dt>Players</dt>
              <dd>{count(d.players)}</dd>
            </div>
            <div>
              <dt>Latest game</dt>
              <dd>{d.latest_game === null ? 'None' : new Date(d.latest_game).toLocaleDateString(undefined, { dateStyle: 'medium' })}</dd>
            </div>
            <div>
              <dt>Last loaded</dt>
              <dd>
                {d.last_load === null
                  ? 'Never'
                  : new Date(d.last_load).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}
                {d.last_load_refused > 0 && <span className="muted"> · {count(d.last_load_refused)} rows refused</span>}
              </dd>
            </div>
          </dl>
          <div className="note-actions">
            <button type="button" className="btn-antique" onClick={() => void takeExport()}>
              Download all data (.zip)
            </button>
            <span className="note-status" role="status">
              {download}
            </span>
          </div>
          {d.unplaced > 0 && (
            <p className="row-error" role="alert">
              {count(d.unplaced)} {d.unplaced === 1 ? 'game is' : 'games are'} in no tournament. A rating run will not start
              until every game is placed.
            </p>
          )}
        </>
      )}
    </section>
  );
}
