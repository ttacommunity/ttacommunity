/** How the admin page reads a rating run's progress line from the engine. */

/** Progress is one engine step line, "[done/total] label | elapsed 2m00s | eta 1h03m00s". */
export type Progress = { done: number; total: number; label: string; elapsed: string; eta: string };

const STEP = /^\[(\d+)\/(\d+)\] (.*?) \| elapsed (\S+) \| eta (\S+)$/;

/** parseProgress reads a step line, with eta '' while unknown, or returns null for any other line. */
export function parseProgress(line: string): Progress | null {
  const m = STEP.exec(line.trim());
  if (m === null) return null;
  const [done, total] = [Number(m[1]), Number(m[2])];
  if (total < 1 || done > total) return null;
  return { done, total, label: m[3], elapsed: m[4], eta: m[5] === '?' ? '' : m[5] };
}

type RunState = { status: string; current: boolean; protected: boolean; deleting: boolean };

/** canDelete says whether a run offers Delete: not protected, not on the site, not going, not already being deleted. */
export function canDelete(run: RunState): boolean {
  return !run.protected && !run.current && !run.deleting && run.status !== 'running';
}

const UNITS = ['B', 'kB', 'MB', 'GB', 'TB'];

/** sizeText writes bytes in the largest unit under a thousand ("4.0 GB"), '' for a size not known yet. */
export function sizeText(bytes: number | undefined): string {
  if (bytes === undefined) return '';
  if (bytes === 0) return '0 B';
  const unit = Math.min(Math.floor(Math.log10(bytes) / 3), UNITS.length - 1);
  return unit === 0 ? `${bytes} B` : `${(bytes / 1000 ** unit).toFixed(1)} ${UNITS[unit]}`;
}

/** runSize writes a run's size, or '' while it is going, as its rows are recorded only when it ends. */
export function runSize(run: { status: string; bytes: number }): string {
  return sizeText(run.status === 'running' ? undefined : run.bytes);
}

/** keepPolling says whether the runs may change without the admin: one is going or being deleted, or a run
 * just started is awaited until waitingUntil (ms since the epoch, as now). */
export function keepPolling(runs: { status: string; deleting: boolean }[], waitingUntil: number, now: number): boolean {
  return now < waitingUntil || runs.some((r) => r.status === 'running' || r.deleting);
}

/** awaitStartedRun gives when to stop waiting for a run started at now to appear: its engine takes a while to connect. */
export function awaitStartedRun(now: number): number {
  return now + 3 * 60 * 1000;
}

type Run = { id: number; status: 'running' | 'complete' | 'failed'; current: boolean; deleting: boolean };
const STATES: Record<Run['status'], string> = { running: 'Running', complete: 'Complete', failed: 'Failed' };

/** runLabel names a run's state: on the site, being deleted, else running, complete or failed. */
export function runLabel(run: Run): string {
  if (run.current) return 'On the site';
  return run.deleting ? 'Deleting…' : STATES[run.status];
}

/** publishVerb says what publishing run does: roll back to an earlier run than the one shown, else publish. */
export function publishVerb(runs: readonly Run[], run: Run): 'Publish' | 'Roll back to' {
  const current = runs.find((r) => r.current);
  return current !== undefined && current.id > run.id ? 'Roll back to' : 'Publish';
}

/** shownRuns returns the latest n runs, or all of them when asked. */
export function shownRuns<T>(runs: readonly T[], all: boolean, n: number): T[] {
  return all ? [...runs] : runs.slice(0, n);
}

/** runGoing says whether a run is going. */
export function runGoing(runs: readonly { status: string }[]): boolean {
  return runs.some((r) => r.status === 'running');
}
