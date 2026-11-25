/** The audit log, for admins: who changed what and when, filtered by action, search and days, a page at a time. */
import { useEffect, useState, type FormEvent } from 'react';
import { formatTime, viewerZone } from '@ttac/shared/time';
import type { Load } from '@ttac/shared/load';
import NewerOlder from '@ttac/shared/NewerOlder';
import { type AdminAuditPage, loadOf } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { actionChoices, auditBy, auditSummary, detailText, searchOf, viewFromSearch, type AuditView } from '../lib/audit-log';
import { Loading } from './Notice';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';

/** AuditPanel opens on the view in the address, else the last seven days in the viewer's zone, and keeps the address on what it shows; a new filter goes back to page 1. */
export default function AuditPanel() {
  const zone = viewerZone();
  const [view, setView] = useState<AuditView>(() => viewFromSearch(new URLSearchParams(window.location.search), new Date(), zone));
  const [text, setText] = useState(view.text);
  const [log, setLog] = useState<Load<AdminAuditPage>>({ kind: 'loading' });
  const audit = req.auditPage(view, zone);
  const path = audit?.path ?? null;

  useEffect(() => {
    if (audit === null) return;
    let live = true;
    setLog({ kind: 'loading' });
    void req.send<AdminAuditPage>(audit).then((r) => {
      if (live) setLog(loadOf(r));
    });
    return () => {
      live = false;
    };
  }, [path]); // a read is named by its path

  const answered = log.kind === 'ready' ? log.data.page : view.page;
  useEffect(() => {
    const { pathname, hash } = window.location;
    window.history.replaceState(null, '', `${pathname}${searchOf({ ...view, page: answered })}${hash}`);
  }, [view, answered]);
  useEffect(
    () => () => {
      const { pathname, hash } = window.location;
      window.history.replaceState(null, '', `${pathname}${hash}`); // another tab's address carries no audit filters
    },
    [],
  );

  const filter = (change: Partial<AuditView>) => setView({ ...view, ...change, page: 1 });
  const search = (e: FormEvent) => {
    e.preventDefault();
    filter({ text });
  };
  const page = log.kind === 'ready' ? log.data : null;

  return (
    <AdminCard id="audit-heading" title="Audit log">
      <p className="muted">
        Every change made in admin, and the changes players make to their own account and blitz games, newest first. Names,
        emails and Discord names are not kept here: an entry names what it changed by id.
      </p>
      <form className="admin-add" onSubmit={search}>
        <label>
          Action
          <select className="input-antique" value={view.action} onChange={(e) => filter({ action: e.target.value })}>
            <option value="">All actions</option>
            {actionChoices(page?.actions, view.action).map((a) => (
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
      {path !== null && <Loading load={log} what="the audit log" />}
      {path !== null && page !== null && (
        <>
          <p className="muted" role="status">
            {auditSummary(page)}
          </p>
          {page.entries.length > 0 && (
            <AdminTable caption="Audit log entries, newest first">
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
            </AdminTable>
          )}
          <NewerOlder page={page.page} pages={page.pages} onPage={(p) => setView({ ...view, page: p })} />
        </>
      )}
    </AdminCard>
  );
}
