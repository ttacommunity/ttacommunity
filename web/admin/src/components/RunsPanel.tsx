/** Rating runs, for admins: start one, watch its progress, compare and publish it, and protect or delete old ones. */
import { Fragment, useEffect, useState } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import * as req from '../lib/admin-requests';
import { adminRequest, type AdminRun, type RunChanges, type StartedRun } from '../lib/admin-api';
import { loadFailure, outcomeOf, refusalOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import Notice, { LoadNotice, NoticeRow } from './Notice';
import { awaitStartedRun, canDelete, changesSummary, keepPolling, parseProgress, publishVerb, runGoing, runLabel, runSize, shownRuns } from '../lib/runs-view';

const POLL_MS = 5000;
const ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone;
const SHOWN = 8;

function when(iso: string | undefined): string {
  return iso === undefined ? '' : new Date(iso).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
}

function ProgressCell({ run }: { run: AdminRun }) {
  const p = parseProgress(run.progress);
  if (p === null) return <span className="muted">{run.progress === '' ? 'Starting…' : run.progress}</span>;
  return (
    <div className="run-progress">
      <progress max={p.total} value={p.done} aria-label={`Run ${run.id}: step ${p.done} of ${p.total}`} />
      <span>
        Step {p.done} of {p.total}: {p.label}
      </span>
      <span className="muted">
        {p.elapsed} so far{p.eta !== '' && `, about ${p.eta} left`}
      </span>
    </div>
  );
}

function Changes({ run }: { run: number }) {
  const [c, setC] = useState<RunChanges | null>(null);
  const [message, setMessage] = useState('');
  useEffect(() => {
    void adminRequest<RunChanges>(`/api/runs/${run}/changes`, {}).then((r) => (r.ok ? setC(r.data) : setMessage(r.message)));
  }, [run]);
  if (message !== '') return <LoadNotice message={message} />;
  if (c === null) return <p className="muted">Comparing…</p>;
  const summary = changesSummary(c);
  if (summary.same) {
    return (
      <div className="run-changes">
        <p>{summary.line}</p>
      </div>
    );
  }
  return (
    <div className="run-changes">
      <h3>
        Run {c.run} against run {c.against}
      </h3>
      <p>
        {c.ratings} ratings (was {c.ratings_before}), {c.new} new, {c.gone} gone.
        {summary.thresholds.length > 0 &&
          ` Moved more than ${summary.thresholds.map((t) => `${t.points} point${t.points === 1 ? '' : 's'}: ${t.count}`).join('; ')}.`}
      </p>
      {c.biggest.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption>Biggest moves, overall conservative rating</caption>
            <thead>
              <tr>
                <th scope="col">Player</th>
                <th scope="col">Before</th>
                <th scope="col">After</th>
                <th scope="col">Change</th>
              </tr>
            </thead>
            <tbody>
              {c.biggest.map((m) => (
                <tr key={m.name}>
                  <th scope="row">{m.name}</th>
                  <td>{m.before.toFixed(0)}</td>
                  <td>{m.after.toFixed(0)}</td>
                  <td>{(m.change > 0 ? '+' : '') + m.change.toFixed(0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** RunsPanel lists the runs and polls while one is going, or just after one was started. */
export default function RunsPanel() {
  const [runs, setRuns] = useState<AdminRun[] | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const close = () => setOutcome(null);
  const dialog = useDialog();
  const [waitingUntil, setWaitingUntil] = useState(0);
  const [comparing, setComparing] = useState<number | null>(null);
  const [all, setAll] = useState(false);

  const load = async () => {
    const r = await adminRequest<AdminRun[]>('/api/runs', {});
    if (r.ok) setRuns(r.data);
    else setOutcome(loadFailure(r.message, 'runs'));
  };
  const going = runs !== null && runGoing(runs);
  const changing = runs !== null && keepPolling(runs, waitingUntil, Date.now());
  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    if (!changing) return;
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [changing, waitingUntil]);

  const start = async () => {
    const sure = await dialog.confirm({
      heading: 'Start a rating run now?',
      text: 'It usually takes 20 to 40 minutes, and is not shown on the site until you publish it.',
      action: 'Start run',
    });
    if (!sure) return;
    setOutcome(null);
    const r = await req.send<StartedRun>(req.startRun());
    if (!r.ok) {
      setOutcome(refused(r.message, 'runs'));
      return;
    }
    setOutcome(succeeded(`Started ${r.data.job}. Its run appears here once the engine connects.`, 'runs'));
    setWaitingUntil(awaitStartedRun(Date.now()));
  };
  const stop = async (run: AdminRun) => {
    const sure = await dialog.confirm({ heading: `Stop run ${run.id}?`, text: 'It is marked failed, and its work so far is not kept.', action: 'Stop run', final: true });
    if (!sure) return;
    setOutcome(null);
    const r = await req.send(req.stopRun(run.id));
    setOutcome(outcomeOf(r, `Stopping run ${run.id}. It shows as failed once the engine has ended.`, 'runs'));
    await load();
  };
  const publish = async (run: AdminRun) => {
    const verb = publishVerb(runs ?? [], run);
    if (!(await dialog.confirm({ heading: `${verb} run ${run.id}?`, text: 'The site shows it at once.', action: `${verb} run ${run.id}` }))) return;
    setOutcome(null);
    const r = await req.send(req.publishRun(run.id));
    setOutcome(outcomeOf(r, `Run ${run.id} is now the one the site shows.`, `run ${run.id}`));
    await load();
  };

  const protect = async (run: AdminRun, on: boolean) => {
    setOutcome(null);
    const r = await req.send(req.protectRun(run.id, on));
    setOutcome(refusalOf(r, `run ${run.id}`));
    await load();
  };
  const remove = async (run: AdminRun) => {
    const size = runSize(run.status, run.disk_bytes);
    const sure = await dialog.confirm({
      heading: `Delete run ${run.id}${size === '' ? '' : ` (${size} on disk)`}?`,
      text: 'Every result of it is deleted too. This cannot be undone.',
      action: 'Delete run',
      final: true,
    });
    if (!sure) return;
    setOutcome(null);
    const r = await req.send(req.deleteRun(run.id));
    setOutcome(outcomeOf(r, `Deleting run ${run.id}. It disappears from this list once it is gone.`, `run ${run.id}`, 'runs'));
    await load();
  };

  const shown = runs === null ? [] : shownRuns(runs, all, SHOWN);
  return (
    <section className="rating-card admin-card" aria-labelledby="runs-heading">
      {dialog.element}
      <h2 id="runs-heading" className="rating-card-heading">
        Rating runs
      </h2>
      <p className="muted">
        A run recomputes every rating from the games. It is not shown on the site until it is published, and an older run
        can be published again to roll back.
      </p>
      <div className="runs-actions">
        <button type="button" className="btn-antique" onClick={() => void start()} disabled={going}>
          {going ? 'A run is going' : 'Start a run now'}
        </button>
      </div>
      <Notice outcome={shownAt(outcome, 'runs')} onClose={close} />
      {runs
        ?.filter((r) => r.status === 'running')
        .map((r) => (
          <div key={r.id} className="run-block">
            <h3>Run {r.id} is going</h3>
            <ProgressCell run={r} />
            <button type="button" className="btn-antique run-stop" onClick={() => void stop(r)}>
              Stop this run
            </button>
          </div>
        ))}
      {runs !== null && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table runs-table">
            <caption className="visually-hidden">Rating runs, newest first</caption>
            <thead>
              <tr>
                <th scope="col">Run</th>
                <th scope="col">State</th>
                <th scope="col">Started ({ZONE})</th>
                <th scope="col" className="run-wide">Games</th>
                <th scope="col" className="run-wide">Players</th>
                <th scope="col">Size</th>
                <th scope="col">On disk</th>
                <th scope="col" className="run-wide">Engine</th>
                <th scope="col">Protect</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => (
                <Fragment key={r.id}>
                  <RunRow
                    run={r}
                    comparing={comparing === r.id}
                    onCompare={() => setComparing(comparing === r.id ? null : r.id)}
                    onPublish={() => void publish(r)}
                    onProtect={(on) => void protect(r, on)}
                    onDelete={() => void remove(r)}
                  />
                  <NoticeRow outcome={shownAt(outcome, `run ${r.id}`)} columns={10} onClose={close} />
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {comparing !== null && (
        <div className="run-block">
          <Changes run={comparing} />
        </div>
      )}
      {runs !== null && runs.length > SHOWN && (
        <button type="button" className="btn-antique" onClick={() => setAll(!all)}>
          {all ? `Show the latest ${SHOWN}` : `Show all ${runs.length}`}
        </button>
      )}
    </section>
  );
}

function RunRow({
  run,
  comparing,
  onCompare,
  onPublish,
  onProtect,
  onDelete,
}: {
  run: AdminRun;
  comparing: boolean;
  onCompare: () => void;
  onPublish: () => void;
  onProtect: (on: boolean) => void;
  onDelete: () => void;
}) {
  const state = run.current ? 'shown' : run.status;
  return (
    <tr>
      <th scope="row">{run.id}</th>
      <td>
        <span className={`run-state run-${state}`}>{runLabel(run)}</span>
      </td>
      <td>{when(run.started_at)}</td>
      <td className="run-wide">{run.games > 0 ? run.games.toLocaleString() : ''}</td>
      <td className="run-wide">{run.players > 0 ? run.players.toLocaleString() : ''}</td>
      <td>{runSize(run.status, run.bytes)}</td>
      <td>{runSize(run.status, run.disk_bytes)}</td>
      <td className="run-wide">{run.engine_version}</td>
      <td>
        <input
          type="checkbox"
          checked={run.protected}
          disabled={run.deleting}
          onChange={(e) => onProtect(e.target.checked)}
          aria-label={`Protect run ${run.id} from deletion`}
        />
      </td>
      <td>
        {canDelete(run) && (
          <button type="button" className="btn-antique run-delete" onClick={onDelete}>
            Delete
          </button>
        )}
        {run.status === 'complete' && !run.deleting && (
          <div className="run-buttons">
            <button type="button" className="btn-antique" aria-expanded={comparing} onClick={onCompare}>
              Changes {comparing ? '▾' : '▸'}
            </button>
            {!run.current && (
              <button type="button" className="btn-antique" onClick={onPublish}>
                Publish
              </button>
            )}
          </div>
        )}
      </td>
    </tr>
  );
}
