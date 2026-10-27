/** The signed-in player's own match in an edition's latest round: where it stands, and reporting its game, correcting their report, or confirming or flagging the opponent's. Nothing for a visitor or a player with no match there. */
import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import { turnMark, turnOrder } from '@ttac/shared/blitz';
import { chooseTurn, gameRecordFrom, TURNS, type GameForm } from '@ttac/shared/game-form';
import { todayIn } from '@ttac/shared/time';
import { browserStorage, seemsSignedIn, sharedMe } from '../lib/account';
import { confirmReport, fetchMyMatch, flagQuestion, flagReport, formFor, myMatchStatus, reportGame, type MyBlitzMatch } from '../lib/blitz-report';

const OUTCOMES: { value: GameForm['a_outcome']; label: string }[] = [
  { value: 'finished', label: 'finished' },
  { value: 'resigned', label: 'resigned' },
  { value: 'timed_out', label: 'timed out' },
];

type Props = { apiUrl: string; slug: string; edition: string; onChanged: () => void; onMatch: (m: MyBlitzMatch | null) => void };

/** BlitzReport shows the player's match once the api names one, tells the page which it is, and refreshes the page's edition after a change. */
export default function BlitzReport({ apiUrl, slug, edition, onChanged, onMatch }: Props) {
  const dialog = useDialog();
  const [match, setMatch] = useState<MyBlitzMatch | null>(null);
  const [zone, setZone] = useState('Unknown');
  const [form, setForm] = useState<GameForm | null>(null);
  const [said, setSaid] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!seemsSignedIn(browserStorage())) return;
    const [me, mine] = await Promise.all([sharedMe(apiUrl), fetchMyMatch(apiUrl, slug, edition)]);
    if (me !== null) setZone(me.zone);
    setMatch(mine);
    onMatch(mine);
  }, [apiUrl, slug, edition, onMatch]);
  useEffect(() => {
    load().catch(() => undefined);
  }, [load]);
  if (match === null) return null;
  const names = { a: match.a, b: match.b ?? '' };
  const done = async (result: { ok: true } | { ok: false; message: string }, text: string) => {
    setBusy(false);
    if (!result.ok) {
      setSaid({ ok: false, text: result.message });
      return;
    }
    setForm(null);
    setSaid({ ok: true, text });
    await load();
    onChanged();
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (form === null) return;
    const r = gameRecordFrom(form, { codeRequired: true });
    if (!r.ok) {
      setSaid({ ok: false, text: r.message });
      return;
    }
    setBusy(true);
    await done(await reportGame(apiUrl, slug, edition, match.id, r.body), 'Reported.');
  };
  const confirm = async () => {
    setBusy(true);
    await done(await confirmReport(apiUrl, slug, edition, match.id), 'Confirmed.');
  };
  const flag = async (opener: HTMLElement) => {
    const reason = await dialog.ask(flagQuestion(match), opener);
    if (reason === null) return;
    setBusy(true);
    await done(await flagReport(apiUrl, slug, edition, match.id, reason), 'Flagged for the organisers.');
  };
  const side = (who: 'a' | 'b') =>
    form !== null && (
      <div className="report-row">
        <label>
          {names[who]}
          <select className="input-antique" value={form[`${who}_outcome`]} onChange={(e) => setForm({ ...form, [`${who}_outcome`]: e.target.value as GameForm['a_outcome'] })}>
            {OUTCOMES.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          {names[who]}&apos;s culture
          <input
            className="input-antique"
            type="number"
            min={0}
            step="any"
            value={form[`${who}_culture`]}
            disabled={form[`${who}_outcome`] !== 'finished'}
            onChange={(e) => setForm({ ...form, [`${who}_culture`]: e.target.value })}
          />
        </label>
        <label>
          {names[who]}&apos;s turn
          <select className="input-antique" value={form[`${who}_turn`] ?? ''} onChange={(e) => setForm(chooseTurn(form, who, Number(e.target.value)))}>
            {form[`${who}_turn`] === null && (
              <option value="" disabled>
                –
              </option>
            )}
            {TURNS.map((t) => (
              <option key={t} value={t}>
                {turnMark(t)}
                {t === 1 ? ' (went first)' : ''}
              </option>
            ))}
          </select>
        </label>
      </div>
    );
  const order = turnOrder(match);
  return (
    <section id="your-game" className="rating-card blitz-card your-game" aria-labelledby="blitz-my-game">
      <h2 id="blitz-my-game" className="rating-card-heading">
        Your game, round {match.round}
      </h2>
      <p className="blitz-intro">
        {match.b === null ? `${names[match.me]}: bye` : `${match.a} v ${match.b}`}. {myMatchStatus(match)}
        {order !== '' && ` ${order}.`}
      </p>
      {form === null && (
        <div className="report-row">
          {match.actions.includes('report') && (
            <button type="button" className="btn-antique" disabled={busy} onClick={() => setForm(formFor(match, zone, todayIn(zone, new Date())))}>
              {match.game === null ? 'Report result' : 'Correct report'}
            </button>
          )}
          {match.actions.includes('confirm') && (
            <button type="button" className="btn-antique" disabled={busy} onClick={() => void confirm()}>
              Confirm
            </button>
          )}
          {match.actions.includes('flag') && (
            <button type="button" className="btn-antique" disabled={busy} onClick={(e) => void flag(e.currentTarget)}>
              Flag as wrong
            </button>
          )}
        </div>
      )}
      {form !== null && (
        <form className="report-form" onSubmit={(e) => void submit(e)}>
          <div className="report-row">
            <label>
              Share code
              <input className="input-antique" value={form.share_code} maxLength={32} required onChange={(e) => setForm({ ...form, share_code: e.target.value })} />
            </label>
            <label>
              Finished on
              <input className="input-antique" type="date" value={form.finished_on} onChange={(e) => setForm({ ...form, finished_on: e.target.value })} />
            </label>
            <label>
              Time ({zone === 'Unknown' ? 'UTC' : zone})
              <input className="input-antique" type="time" value={form.finished_time} onChange={(e) => setForm({ ...form, finished_time: e.target.value })} />
            </label>
          </div>
          {side('a')}
          {side('b')}
          <div className="report-row">
            <button type="submit" className="btn-antique" disabled={busy}>
              Save
            </button>
            <button type="button" className="btn-antique" disabled={busy} onClick={() => setForm(null)}>
              Cancel
            </button>
          </div>
        </form>
      )}
      {said !== null && (
        <p className={said.ok ? 'report-said' : 'report-said report-refused'} role="status">
          {said.text}
        </p>
      )}
      {dialog.element}
    </section>
  );
}
