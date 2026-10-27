/** The leaderboard choices a browser remembers between visits, as upstream's session does. */
import { DEFAULT_VIEW, parseView, viewParams, type Remembered, type View } from './leaderboard-view';

const KEY = 'ttac.leaderboard';
const PARAMS = { engine: 'model', reset: 'reset_mode', format: 'format' } as const;

/** remember saves the view's engine, reset mode and format; a refusing store, or none, is ignored. */
export function remember(store: Storage | null, view: View): void {
  if (store === null) return;
  const all = viewParams({ ...DEFAULT_VIEW, engine: view.engine, reset: view.reset, format: view.format }, true);
  try {
    store.setItem(KEY, all.toString());
  } catch {
    // Private windows and blocked storage refuse; the page works without memory.
  }
}

/** recall returns the remembered choices that are still valid, or none (also without a store). */
export function recall(store: Storage | null): Remembered {
  if (store === null) return {};
  let saved: string | null = null;
  try {
    saved = store.getItem(KEY);
  } catch {
    return {};
  }
  const params = new URLSearchParams(saved ?? '');
  const parsed = parseView(params, {});
  const got: Remembered = {};
  for (const [field, param] of Object.entries(PARAMS) as [keyof typeof PARAMS, string][]) {
    const raw = params.get(param);
    const value = parsed[field];
    if (raw !== null && String(value) === raw) Object.assign(got, { [field]: value });
  }
  return got;
}
