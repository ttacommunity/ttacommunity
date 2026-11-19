/** A season's sign-ups in admin: open or close them, with a closing time or none, and enter or remove the sign-ups waiting for an account to be linked. Any tournament that takes sign-ups places it. */
import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import { viewerZone } from '@ttac/shared/time';
import { adminRequest } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import type { DeadlineForm } from '../lib/blitz-deadlines';
import { outcomeOf, refused, shownAt, type Outcome } from '../lib/outcome';
import { canAccept, openBlocker, pendingNote, signUpsSummary, type AdminSignUps } from '../lib/sign-ups';
import type { Region } from '../lib/zones';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';
import Count from './Count';
import Notice from './Notice';
import ZonePick from './ZonePick';

type Props = {
  /** The tournament and season whose sign-ups these are. */
  slug: string;
  season: string;
  /** Whether the site shows the tournament, and how many rounds are paired: opening needs it shown and none paired. */
  shown: boolean;
  rounds: number;
  regions: readonly Region[];
  /** Called after a sign-up is entered, so the page shows the new entrant. */
  onEntered: () => Promise<void>;
};

/** SignUpsPanel loads the season's sign-ups and changes them, saying how each change went. */
export default function SignUpsPanel({ slug, season, shown, rounds, regions, onEntered }: Props) {
  const dialog = useDialog();
  const [data, setData] = useState<AdminSignUps | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [closes, setCloses] = useState<DeadlineForm | null>(null);
  const zone = viewerZone();
  const load = useCallback(async () => {
    const r = await adminRequest<AdminSignUps>(`/api/tournaments/${slug}/seasons/${encodeURIComponent(season)}/sign-ups`, {});
    if (r.ok) setData(r.data);
    else setOutcome(refused(r.message, 'sign-ups'));
  }, [slug, season]);
  useEffect(() => {
    void load();
  }, [load]);
  const run = async (r: req.Request, done: string, entered = false) => {
    const sent = await req.send(r);
    setOutcome(outcomeOf(sent, done, 'sign-ups'));
    if (sent.ok) setCloses(null);
    await load();
    if (sent.ok && entered) await onEntered();
  };
  const open = (e: FormEvent) => {
    e.preventDefault();
    const at = closes !== null && closes.day !== '' ? closes : null;
    void run(req.setSignUps(slug, season, true, at), at === null ? 'Sign-ups open, with no closing time.' : 'Sign-ups open.');
  };
  const close = async (opener: HTMLElement) => {
    const sure = await dialog.confirm(
      { heading: 'Close sign-ups?', text: 'Players can no longer sign up or withdraw on the site. You can open them again; that posts to Discord again.', action: 'Close sign-ups' },
      opener,
    );
    if (sure) await run(req.setSignUps(slug, season, false, null), 'Sign-ups closed.');
  };
  const remove = async (id: number, name: string, opener: HTMLElement) => {
    const sure = await dialog.confirm({ heading: `Remove ${name}'s sign-up?`, text: 'Their request to be linked stays, for the Members tab to answer.', action: 'Remove' }, opener);
    if (sure) await run(req.removeSignUp(slug, season, id), `${name}'s sign-up removed.`);
  };
  const blocker = openBlocker(shown, rounds);
  return (
    <AdminCard
      id="sign-ups-title"
      title={
        <>
          Sign-ups <Count n={data?.pending.length ?? 0} what="waiting" />
        </>
      }
    >
      {dialog.element}
      <Notice outcome={shownAt(outcome, 'sign-ups')} onClose={() => setOutcome(null)} />
      {data !== null && (
        <>
          <p className="season-summary">
            <span className="muted">{signUpsSummary(data.sign_ups, zone)}</span>{' '}
            {!data.sign_ups.open && (
              <button type="button" className="btn-antique btn-settings" disabled={blocker !== ''} aria-expanded={closes !== null} onClick={() => setCloses(closes === null ? { day: '', time: '', zone } : null)}>
                Open sign-ups
              </button>
            )}
            {data.sign_ups.open && (
              <>
                <button type="button" className="btn-antique btn-settings" aria-expanded={closes !== null} onClick={() => setCloses(closes === null ? { day: '', time: '', zone } : null)}>
                  Change closing time
                </button>{' '}
                <button type="button" className="btn-antique btn-settings" onClick={(e) => void close(e.currentTarget)}>
                  Close sign-ups
                </button>
              </>
            )}
            {!data.sign_ups.open && blocker !== '' && <span className="row-detail muted">{blocker}</span>}
          </p>
          {closes !== null && (
            <form className="result-row" onSubmit={open}>
              <label>
                Closes on
                <input className="input-antique" type="date" value={closes.day} onChange={(e) => setCloses({ ...closes, day: e.target.value })} />
              </label>
              <label>
                Time
                <input className="input-antique" type="time" required={closes.day !== ''} value={closes.time} onChange={(e) => setCloses({ ...closes, time: e.target.value })} />
              </label>
              <ZonePick label="Time zone" regions={regions} value={closes.zone} onChange={(z) => setCloses({ ...closes, zone: z })} />
              <button type="submit" className="btn-antique">
                {data.sign_ups.open ? 'Save' : 'Open'}
              </button>
              <button type="button" className="btn-antique" onClick={() => setCloses(null)}>
                Cancel
              </button>
              <span className="row-detail muted">Leave the day empty for no closing time: sign-ups then close at round 1, or when you close them.{data.sign_ups.open ? '' : ' Opening them posts to the Discord channels taking Blitz sign-ups.'}</span>
            </form>
          )}
          {data.pending.length > 0 && (
            <AdminTable caption="Sign-ups waiting for their account to be linked, oldest first">
              <thead>
                <tr>
                  <th scope="col">TTA username given</th>
                  <th scope="col">Where it stands</th>
                  <th scope="col">
                    <span className="visually-hidden">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.pending.map((p) => (
                  <tr key={p.id}>
                    <th scope="row">{p.name}</th>
                    <td className="muted">{pendingNote(p, zone)}</td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" disabled={!canAccept(p)} onClick={() => void run(req.acceptSignUp(slug, season, p.id), `${p.name} entered.`, true)}>
                        Accept
                      </button>
                      <button type="button" className="btn-antique" onClick={(e) => void remove(p.id, p.name, e.currentTarget)}>
                        Remove
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </AdminTable>
          )}
        </>
      )}
    </AdminCard>
  );
}
