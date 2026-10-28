/** The audit log, for admins: who changed what and when, filtered by action, search and days, a page at a time. */
import { useEffect, useState, type FormEvent } from 'react';
import { formatTime, viewerZone } from '@ttac/shared/time';
import type { Load } from '@ttac/shared/load';
import { adminRequest, type AdminAuditPage } from '../lib/admin-api';
import { auditBy, auditPath, auditSummary, defaultView, detailText, type AuditView } from '../lib/audit-log';

/** AuditPanel opens on the last seven days in the viewer's zone; a new filter goes back to page 1. */
export default function AuditPanel() {
  const zone = viewerZone();
  const [view, setView] = useState<AuditView>(() => defaultView(new Date(), zone));
  const [text, setText] = useState('');
  const [log, setLog] = useState<Load<AdminAuditPage>>({ kind: 'loading' });
  const path = auditPath(view, zone);

  useEffect(() => {
    if (path === null) return;
    let live = true;
    setLog({ kind: 'loading' });
    void adminRequest<AdminAuditPage>(path, {}).then((r) => {
      if (live) setLog(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
    });
    return () => {
      live = false;
    };
  }, [path]);

  const filter = (change: Partial<AuditView>) => setView({ ...view, ...change, page: 1 });
  const search = (e: FormEvent) => {
    e.preventDefault();
    filter({ text });
  };
  const page = log.kind === 'ready' ? log.data : null;

  return (
    <section className="rating-card admin-card" aria-labelledby="audit-heading">
      <h2 id="audit-heading" className="rating-card-heading">
        Audit log
      </h2>
      <p className="muted">
        Every change made in admin, and the changes players make to their own account and blitz games, newest first. Names,
        emails and Discord names are not kept here: an entry names what it changed by id.
      </p>
      <form className="admin-add" onSubmit={search}>
        <label>
          Action
          <select className="input-antique" value={view.action} onChange={(e) => filter({ action: e.target.value })}>
            <option value="">All actions</option>
            {(page?.actions ?? (view.action === '' ? [] : [view.action])).map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
        <label>
          From
          <input className="input-antique" type="date" value={view.from} onChange={(e) => filter({ from: e.target.value })} />
        </label>
        <label>
          To
          <input className="input-antique" type="date" value={view.to} onChange={(e) => filter({ to: e.target.value })} />
        </label>
        <label>
          Search
          <input className="input-antique" type="search" maxLength={100} value={text} onChange={(e) => setText(e.target.value)} />
        </label>
        <button type="submit" className="btn-antique">
          Search
        </button>
      </form>
      {path === null && <p role="alert">The From day is after the To day.</p>}
      {path !== null && log.kind === 'loading' && <p className="muted">Loading the audit log…</p>}
      {path !== null && log.kind === 'error' && <p role="alert">{log.message}</p>}
      {path !== null && page !== null && (
        <>
          <p className="muted" role="status">
            {auditSummary(page)}
          </p>
          {page.entries.length > 0 && (
            <div className="table-responsive">
              <table className="leaderboard-table admin-table">
                <caption className="visually-hidden">Audit log entries, newest first</caption>
                <thead>
                  <tr>
                    <th scope="col">When ({zone})</th>
                    <th scope="col">Who</th>
                    <th scope="col">Action</th>
                    <th scope="col">Target</th>
                    <th scope="col">Detail</th>
                  </tr>
                </thead>
                <tbody>
                  {page.entries.map((e) => (
                    <tr key={e.id}>
                      <td>{formatTime(e.at, zone)}</td>
                      <td>{auditBy(e)}</td>
                      <td>{e.action}</td>
                      <td className="admin-id">{e.target}</td>
                      <td>{detailText(e.detail)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {page.pages > 1 && (
            <div className="note-actions">
              <button type="button" className="btn-antique" disabled={page.page <= 1} onClick={() => setView({ ...view, page: page.page - 1 })}>
                Newer
              </button>
              <button
                type="button"
                className="btn-antique"
                disabled={page.page >= page.pages}
                onClick={() => setView({ ...view, page: page.page + 1 })}
              >
                Older
              </button>
            </div>
          )}
        </>
      )}
    </section>
  );
}
