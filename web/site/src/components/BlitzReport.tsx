/** The signed-in player's own match in an edition's latest round: where it stands, game by game, and reporting a game, correcting their report, confirming or flagging the opponent's, or agreeing best of 3. Nothing for a visitor or a player with no match there. */
import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import { turnOrder } from '@ttac/shared/blitz';
import { gameRecordFrom, type GameForm } from '@ttac/shared/game-form';
import GameSeatFields from '@ttac/shared/GameSeatFields';
import { todayIn } from '@ttac/shared/time';
import { seemsSignedIn, sharedMe } from '../lib/account';
import { browserStorage } from '@ttac/shared/storage';
import { answerBestOf, BEST_OF_LABELS, bestOfStatus, confirmReport, fetchMyBlitz, flagQuestion, flagReport, formFor, reportGame, reportLines, type BestOfAction, type MyBlitz, type MyBlitzMatch, type ReportLine } from '../lib/blitz-report';
import OutcomeNotice from '@ttac/shared/OutcomeNotice';
import Panel from './Panel';

type Props = { apiUrl: string; slug: string; edition: string; onChanged: () => void; onSeen: (mine: MyBlitz | null) => void };

/** BlitzReport shows the player's match once the api names one, tells the page what the api says to this viewer (null for nobody signed in), and refreshes the page's edition after a change. */
export default function BlitzReport({ apiUrl, slug, edition, onChanged, onSeen }: Props) {
  const dialog = useDialog();
  const [match, setMatch] = useState<MyBlitzMatch | null>(null);
  const [zone, setZone] = useState('Unknown');
  const [form, setForm] = useState<GameForm | null>(null);
  const [number, setNumber] = useState(1);
  const [said, setSaid] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    if (!seemsSignedIn(browserStorage())) {
      onSeen(null);
      return;
    }
    const [me, mine] = await Promise.all([sharedMe(apiUrl), fetchMyBlitz(apiUrl, slug, edition)]);
    if (me !== null) setZone(me.zone);
    setMatch(mine?.match ?? null);
    onSeen(mine);
  }, [apiUrl, slug, edition, onSeen]);
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
    await done(await reportGame(apiUrl, slug, edition, match.id, number, r.body), 'Reported.');
  };
  const confirm = async (n: number) => {
    setBusy(true);
    await done(await confirmReport(apiUrl, slug, edition, match.id, n), 'Confirmed.');
  };
  const flag = async (n: number, opener: HTMLElement) => {
    const reason = await dialog.ask(flagQuestion(match), opener);
    if (reason === null) return;
    setBusy(true);
    await done(await flagReport(apiUrl, slug, edition, match.id, n, reason), 'Flagged for the organisers.');
  };
  const answer = async (a: BestOfAction) => {
    setBusy(true);
    await done(await answerBestOf(apiUrl, slug, edition, match.id, a), a === 'decline' ? 'Declined: one game.' : 'Saved.');
  };
  const open = (l: ReportLine) => {
    setNumber(l.number);
    setForm(formFor(l.game, zone, todayIn(zone, new Date())));
  };
  const side = (who: 'a' | 'b') =>
    form !== null && (
      <div className="report-row">
        <GameSeatFields form={form} who={who} name={names[who]} onChange={setForm} />
      </div>
    );
  const lines = reportLines(match);
  const oneGame = match.best_of === 1;
  const orderOf = (l: ReportLine) => turnOrder({ a: match.a, b: match.b, result: null, game: l.game, ruled: false });
  const bestOf = bestOfStatus(match);
  const buttons = (l: ReportLine) =>
    form === null &&
    l.actions.length > 0 && (
      <div className="report-row">
        {l.actions.includes('report') && (
          <button type="button" className="btn-antique" disabled={busy} onClick={() => open(l)}>
            {oneGame ? (l.game === null ? 'Report result' : 'Correct report') : l.game === null ? `Report game ${l.number}` : `Correct game ${l.number}`}
          </button>
        )}
        {l.actions.includes('confirm') && (
          <button type="button" className="btn-antique" disabled={busy} onClick={() => void confirm(l.number)}>
            Confirm
          </button>
        )}
        {l.actions.includes('flag') && (
          <button type="button" className="btn-antique" disabled={busy} onClick={(e) => void flag(l.number, e.currentTarget)}>
            Flag as wrong
          </button>
        )}
      </div>
    );
  const first = lines[0];
  return (
    <Panel id="your-game" title={`Your game, round ${match.round}`}>
      <p className="blitz-intro">
        {match.b === null ? `${names[match.me]}: bye` : `${match.a} v ${match.b}`}. {oneGame ? first.text : bestOf}
        {oneGame && orderOf(first) !== '' && ` ${orderOf(first)}.`}
      </p>
      {oneGame && bestOf !== '' && <p className="blitz-intro">{bestOf}</p>}
      {form === null && match.best_of_actions.length > 0 && (
        <div className="report-row">
          {match.best_of_actions.map((a) => (
            <button key={a} type="button" className="btn-antique" disabled={busy} onClick={() => void answer(a)}>
              {BEST_OF_LABELS[a]}
            </button>
          ))}
        </div>
      )}
      {oneGame && buttons(first)}
      {!oneGame &&
        lines.map((l) => (
          <div key={`${l.number}-${l.heading}`} className="report-game">
            <p className="blitz-intro">
              {l.heading !== '' && <strong>{l.heading}: </strong>}
              {l.text}
              {orderOf(l) !== '' && ` ${orderOf(l)}.`}
            </p>
            {buttons(l)}
          </div>
        ))}
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
      {said !== null && <OutcomeNotice kind={said.ok ? 'done' : 'refused'} text={said.text} onClose={() => setSaid(null)} />}
      {dialog.element}
    </Panel>
  );
}
