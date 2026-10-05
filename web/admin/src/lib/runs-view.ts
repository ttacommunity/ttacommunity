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
