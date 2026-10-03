/** Rating runs, for admins: start one now, watch its progress, compare it with the published run, and publish it. */
import { useEffect, useState } from 'react';
import { adminRequest, type AdminRun, type RunChanges, type StartedRun } from '../lib/admin-api';
import { parseProgress } from '../lib/runs-view';

const POLL_MS = 5000;
const ZONE = Intl.DateTimeFormat().resolvedOptions().timeZone;
const SHOWN = 8;
const STATES: Record<AdminRun['status'], string> = { running: 'Running', complete: 'Complete', failed: 'Failed' };

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
  if (message !== '') return <p className="row-error">{message}</p>;
  if (c === null) return <p className="muted">Comparing…</p>;
  return (
    <div className="run-changes">
      <p>
        Run {c.against}: {c.ratings} ratings (was {c.ratings_before}), {c.new} new, {c.gone} gone. Moved more than
        1 point: {c.moved_1}; 10: {c.moved_10}; 50: {c.moved_50}; 100: {c.moved_100}.
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
  const [message, setMessage] = useState('');
  const [waitingUntil, setWaitingUntil] = useState(0);
  const [comparing, setComparing] = useState<number | null>(null);
  const [all, setAll] = useState(false);

  const load = async () => {
    const r = await adminRequest<AdminRun[]>('/api/runs', {});
    if (r.ok) setRuns(r.data);
    else setMessage(r.message);
  };
  const going = runs?.some((r) => r.status === 'running') ?? false;
  useEffect(() => {
    void load();
  }, []);
  useEffect(() => {
    if (!going && Date.now() > waitingUntil) return;
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [going, waitingUntil]);

  const start = async () => {
    if (!window.confirm('Start a rating run now? It usually takes 20 to 40 minutes, and is not shown on the site until you publish it.')) return;
    setMessage('');
    const r = await adminRequest<StartedRun>('/api/runs', { method: 'POST' });
    if (!r.ok) {
      setMessage(r.message);
      return;
    }
    setMessage(`Started ${r.data.job}. Its run appears here once the engine connects.`);
    setWaitingUntil(Date.now() + 3 * 60 * 1000);
  };
  const stop = async (run: AdminRun) => {
    if (!window.confirm(`Stop run ${run.id}? It is marked failed, and its work so far is not kept.`)) return;
    setMessage('');
    const r = await adminRequest('/api/runs/stop', { method: 'POST' });
    setMessage(r.ok ? `Stopping run ${run.id}. It shows as failed once the engine has ended.` : r.message);
    await load();
  };
  const publish = async (run: AdminRun) => {
    const current = runs?.find((r) => r.current);
    const verb = current !== undefined && current.id > run.id ? 'Roll back to' : 'Publish';
    if (!window.confirm(`${verb} run ${run.id}? The site shows it at once.`)) return;
    setMessage('');
    const r = await adminRequest(`/api/runs/${run.id}/publish`, { method: 'POST' });
    setMessage(r.ok ? `Run ${run.id} is now the one the site shows.` : r.message);
    await load();
  };

  const shown = runs === null ? [] : all ? runs : runs.slice(0, SHOWN);
  return (
    <section className="rating-card admin-card" aria-labelledby="runs-heading">
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
      {message !== '' && (
        <p className="admin-message" role="status">
          {message}
        </p>
      )}
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
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => (
                <RunRow
                  key={r.id}
                  run={r}
                  comparing={comparing === r.id}
                  onCompare={() => setComparing(comparing === r.id ? null : r.id)}
                  onPublish={() => void publish(r)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {comparing !== null && (
        <div className="run-block">
          <h3>Run {comparing} against the published run</h3>
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
}: {
  run: AdminRun;
  comparing: boolean;
  onCompare: () => void;
  onPublish: () => void;
}) {
  const state = run.current ? 'shown' : run.status;
  return (
    <tr>
      <th scope="row">{run.id}</th>
      <td>
        <span className={`run-state run-${state}`}>{run.current ? 'On the site' : STATES[run.status]}</span>
      </td>
      <td>{when(run.started_at)}</td>
      <td className="run-wide">{run.games > 0 ? run.games.toLocaleString() : ''}</td>
      <td className="run-wide">{run.players > 0 ? run.players.toLocaleString() : ''}</td>
      <td>
        {run.status === 'complete' && !run.current && (
          <div className="run-buttons">
            <button type="button" className="btn-antique" aria-expanded={comparing} onClick={onCompare}>
              Changes {comparing ? '▾' : '▸'}
            </button>
            <button type="button" className="btn-antique" onClick={onPublish}>
              Publish
            </button>
          </div>
        )}
      </td>
    </tr>
  );
}
