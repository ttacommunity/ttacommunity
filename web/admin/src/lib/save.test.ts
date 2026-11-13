import { describe, expect, it } from 'vitest';
import type { AdminResult } from './admin-api';
import { runSave, statusText, type SaveState } from './save';

const OK: AdminResult<{ moved: number }> = { ok: true, data: { moved: 3 } };
const NO: AdminResult<{ moved: number }> = { ok: false, status: 409, message: 'Refused: that name is taken.' };

/** states runs a save and returns every state it set, and its result. */
async function states(send: (() => Promise<AdminResult<{ moved: number }>>) | null, done: string | ((d: { moved: number }) => string), working?: string) {
  const seen: SaveState[] = [];
  const r = await runSave(send, { done, working }, (s) => seen.push(s));
  return { seen, r };
}

describe('runSave', () => {
  it.each([
    ['done: its word', OK, 'Saved', [{ status: 'Saving…', refusal: null }, { status: 'Saved', refusal: null }]],
    ['refused: no word, and why', NO, 'Saved', [{ status: 'Saving…', refusal: null }, { status: '', refusal: { kind: 'refused', text: 'Refused: that name is taken.', at: '' } }]],
  ])('ends %s', async (_, result, done, want) => {
    const { seen } = await states(() => Promise.resolve(result), done);
    expect(seen).toEqual(want);
  });

  it('with nothing to save sets nothing, so no Saving… stays behind', async () => {
    const { seen, r } = await states(null, 'Saved');
    expect(seen).toEqual([]);
    expect(r).toBeNull();
  });

  it('says its own word while working', async () => {
    const { seen } = await states(() => Promise.resolve(OK), 'Moved', 'Moving…');
    expect(seen[0].status).toBe('Moving…');
  });

  it('words what was done from the answer', async () => {
    const { seen } = await states(() => Promise.resolve(OK), (d) => `Moved ${d.moved} entries.`);
    expect(seen.at(-1)?.status).toBe('Moved 3 entries.');
  });

  it('returns the result, so the caller can act on it', async () => {
    const { r } = await states(() => Promise.resolve(NO), 'Saved');
    expect(r).toEqual(NO);
  });
});

describe('statusText', () => {
  it.each([
    ['the word alone', '', 'Saved', 'Saved'],
    ['nothing', '', '', ''],
    ['the prefix alone', '3 chosen', '', '3 chosen'],
    ['both', '3 chosen', 'Saving…', '3 chosen · Saving…'],
  ])('writes %s', (_, prefix, status, want) => {
    expect(statusText(prefix, status)).toBe(want);
  });
});
