/** Players' sign-ins on the Members tab: requests to be linked, approved or refused by admins and organisers, and sign-ins linked to no one, removed by admins. */
import { Fragment } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import type { AdminLogin, AdminMe, LinkApproved, Load } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import { detailsHint, requestHint, standing } from '../lib/sign-ins';
import Notice, { LoadNotice, NoticeRow } from './Notice';

/** who names a sign-in: its Discord username, with the display name when it differs. */
export function who(l: AdminLogin): string {
  return l.display_name === '' || l.display_name === l.username ? l.username : `${l.username} (${l.display_name})`;
}

/** when shows an instant in the viewer's zone, as admin's tables do. */
function when(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

/** Said is the Members tab's outcome, shared so each shows where it belongs, and after reloads what an answer changes. */
type Said = { outcome: Outcome | null; say: (o: Outcome | null) => void; after: () => Promise<void> };

/** WaitingRequests lists the requests to be linked, oldest first, for an admin or organiser to approve or refuse. */
export function WaitingRequests({ logins, waiting, outcome, say, after }: { logins: Load<unknown>; waiting: AdminLogin[] } & Said) {
  const close = () => say(null);
  const dialog = useDialog();
  const approve = async (l: AdminLogin, request: number) => {
    const r = await req.send<LinkApproved>(req.approveLink(request));
    const done = r.ok ? `Linked ${l.username} to ${r.data.member}${r.data.created ? ', a new member' : ''}.` : '';
    say(outcomeOf(r, done, `login ${l.id}`, 'waiting'));
    await after();
  };
  const refuse = async (l: AdminLogin, request: number) => {
    const reason = await dialog.ask({
      heading: `Refuse ${l.username}'s request?`,
      text: 'They see the reason, and may ask again.',
      field: 'The reason',
      required: false,
      maxLength: 500,
      action: 'Refuse',
    });
    if (reason === null) return;
    say(outcomeOf(await req.send(req.refuseLink(request, reason)), `Refused ${l.username}'s request.`, `login ${l.id}`, 'waiting'));
    await after();
  };
  return (
    <div className="admin-subsection">
      {dialog.element}
      <h3>
        Waiting to be linked
        {waiting.length > 0 && <span className="count-badge">{waiting.length}</span>}
      </h3>
      <p className="muted">Check a request is really that player before approving it.</p>
      <Notice outcome={shownAt(outcome, 'waiting')} onClose={close} />
      {logins.kind === 'loading' && <p className="muted">Loading…</p>}
      {logins.kind === 'error' && <LoadNotice message={logins.message} />}
      {logins.kind === 'ready' && waiting.length === 0 && <p className="muted">No requests waiting.</p>}
      {waiting.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Requests to be linked, oldest first</caption>
            <thead>
              <tr>
                <th scope="col">Discord</th>
                <th scope="col">Asks to be</th>
                <th scope="col">Asked</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {waiting.map((l) => {
                const r = l.request;
                if (r === null) return null;
                return (
                  <Fragment key={l.id}>
                    <tr>
                      <th scope="row">{who(l)}</th>
                      <td>
                        {r.name}
                        <span className="row-detail muted">{requestHint(r, l.username).text}</span>
                        <span className="row-detail muted">{detailsHint(r).text}</span>
                      </td>
                      <td>{when(r.requested_at)}</td>
                      <td className="run-buttons">
                        <button type="button" className="btn-antique" onClick={() => void approve(l, r.id)}>
                          Approve
                        </button>
                        <button type="button" className="btn-antique" onClick={() => void refuse(l, r.id)}>
                          Refuse
                        </button>
                      </td>
                    </tr>
                    <NoticeRow outcome={shownAt(outcome, `login ${l.id}`)} columns={4} onClose={close} />
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** UnlinkedSignIns lists the sign-ins linked to no one and not waiting: never asked, or refused; an admin may remove one. */
export function UnlinkedSignIns({ unlinked, role, outcome, say, after }: { unlinked: AdminLogin[]; role: AdminMe['role'] } & Said) {
  const close = () => say(null);
  const dialog = useDialog();
  const remove = async (l: AdminLogin) => {
    const sure = await dialog.confirm({
      heading: `Remove ${l.username}'s sign-in?`,
      text: 'They are signed out everywhere, and their requests go.',
      action: 'Remove',
      final: true,
    });
    if (!sure) return;
    say(outcomeOf(await req.send(req.removeLogin(l.id)), `Removed ${l.username}'s sign-in.`, `login ${l.id}`, 'unlinked'));
    await after();
  };
  return (
    <div className="admin-subsection">
      {dialog.element}
      <h3>Sign-ins not linked</h3>
      <p className="muted">Signed in, but not linked to a member and not waiting for an answer.</p>
      <Notice outcome={shownAt(outcome, 'unlinked')} onClose={close} />
      {unlinked.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Sign-ins not linked to a member</caption>
            <thead>
              <tr>
                <th scope="col">Discord</th>
                <th scope="col">Where it stands</th>
                <th scope="col">Last signed in</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {unlinked.map((l) => {
                const s = standing(l);
                return (
                  <Fragment key={l.id}>
                    <tr>
                      <th scope="row">{who(l)}</th>
                      <td>
                        {l.refused === null ? <span className="muted">{s.text}</span> : s.text}
                        {s.detail !== '' && <span className="row-detail muted">{s.detail}</span>}
                      </td>
                      <td>{when(l.signed_in_at)}</td>
                      <td className="run-buttons">
                        {role === 'admin' && (
                          <button type="button" className="btn-antique" onClick={() => void remove(l)}>
                            Remove
                          </button>
                        )}
                      </td>
                    </tr>
                    <NoticeRow outcome={shownAt(outcome, `login ${l.id}`)} columns={4} onClose={close} />
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
