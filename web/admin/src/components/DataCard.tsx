/** The loaded data at a glance: games, players, and when they were last loaded. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import { formatDay, formatInstant, viewerZone } from '@ttac/shared/time';
import { adminDownload, adminRequest, exportFileName } from '../lib/admin-api';
import { refusalOf, type Outcome } from '../lib/outcome';
import { sizeText } from '../lib/runs-view';
import Notice, { LoadNotice } from './Notice';

type DataSummary = components['schemas']['DataSummary'];

const count = (n: number) => n.toLocaleString();

/** DataCard loads the summary once and shows it, warning when games sit in no tournament. */
export default function DataCard() {
  const [d, setD] = useState<DataSummary | null>(null);
  const [message, setMessage] = useState('');
  const [download, setDownload] = useState('');
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const takeExport = async () => {
    setDownload('Preparing the download…');
    setRefusal(null);
    const r = await adminDownload('/api/export');
    setRefusal(refusalOf(r));
    if (!r.ok) {
      setDownload('');
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
      {message !== '' && <LoadNotice message={message} />}
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
              <dd>{d.latest_game === null ? 'None' : formatDay(d.latest_game)}</dd>
            </div>
            <div>
              <dt>Last loaded</dt>
              <dd>
                {d.last_load === null
                  ? 'Never'
                  : formatInstant(d.last_load, viewerZone())}
                {d.last_load_refused > 0 && <span className="muted"> · {count(d.last_load_refused)} rows refused</span>}
              </dd>
            </div>
            <div>
              <dt>Database</dt>
              <dd>
                {sizeText(d.database_bytes)}
                <span className="muted"> · rating runs {sizeText(d.runs_disk_bytes)}</span>
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
          <Notice outcome={refusal} onClose={() => setRefusal(null)} />
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
