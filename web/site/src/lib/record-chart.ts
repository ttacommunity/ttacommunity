/** The record history's chart: where each record sits on a step line through time, its axes, and the entries that do not count. */
import { niceTicks } from './challenges';

/** The chart's size in its own units, and the margins its axes take. */
export const CHART = { width: 800, height: 220, top: 16, right: 16, bottom: 28, left: 48 };

interface Step {
  entered_at: string;
  domination: number;
}

/** RecordChart is a step line's path, each record's point, the value ticks with their height, and each new year's tick with its place. */
export interface RecordChart {
  path: string;
  right: number;
  points: { x: number; y: number }[];
  yTicks: { value: number; y: number }[];
  xTicks: { year: number; x: number }[];
}

/** recordChart lays out records, oldest first, from the first to now: each held flat until the next. */
export function recordChart(steps: readonly Step[], now: number): RecordChart {
  const times = steps.map((s) => Date.parse(s.entered_at));
  const t0 = times[0];
  const t1 = Math.max(now, times[times.length - 1] + 1);
  const ticks = niceTicks(steps[0].domination, steps[steps.length - 1].domination);
  const yMin = ticks[0];
  const yMax = ticks[ticks.length - 1] > yMin ? ticks[ticks.length - 1] : yMin + 1;
  const x = (t: number) => CHART.left + ((t - t0) / (t1 - t0 || 1)) * (CHART.width - CHART.left - CHART.right);
  const y = (v: number) => CHART.height - CHART.bottom - ((v - yMin) / (yMax - yMin)) * (CHART.height - CHART.top - CHART.bottom);

  const points = steps.map((s, i) => ({ x: x(times[i]), y: y(s.domination) }));
  let path = `M${points[0].x},${points[0].y}`;
  for (const p of points.slice(1)) path += `H${p.x}V${p.y}`;
  const right = x(t1);
  path += `H${right}`;

  const xTicks = [];
  for (let year = new Date(t0).getUTCFullYear() + 1; Date.UTC(year, 0, 1) < t1; year++) xTicks.push({ year, x: x(Date.UTC(year, 0, 1)) });
  return { path, right, points, yTicks: ticks.map((value) => ({ value, y: y(value) })), xTicks };
}

/** notCounted are the bugged entries the chosen entries would show, highest first: listed, never counted. */
export function notCounted<E extends { domination: number; proof_url: string | null; bugged: boolean }>(entries: readonly E[], verifiedOnly: boolean): E[] {
  return entries.filter((e) => e.bugged && (!verifiedOnly || e.proof_url !== null)).sort((a, b) => b.domination - a.domination);
}
