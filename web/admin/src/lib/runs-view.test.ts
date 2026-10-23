import { describe, expect, it } from 'vitest';
import { awaitStartedRun, canDelete, changesSummary, keepPolling, parseProgress, publishVerb, runGoing, runLabel, runSize, shownRuns, sizeText } from './runs-view';

describe('parseProgress', () => {
  it('reads the step, label, time taken and time left from an engine line', () => {
    expect(parseProgress('[1/135] still on 133 steps in 4 workers | elapsed 2m00s | eta 2h23m18s')).toEqual({
      done: 1,
      total: 135,
      label: 'still on 133 steps in 4 workers',
      elapsed: '2m00s',
      eta: '2h23m18s',
    });
  });

  it('leaves the time left empty when the engine does not know it yet', () => {
    expect(parseProgress('[0/1] still on pairs | elapsed 0m30s | eta ?')).toMatchObject({ done: 0, total: 1, eta: '' });
  });

  it.each([
    ['an empty line', ''],
    ['a run header', 'run 33: community, engine 218f9a35'],
    ['a step out of none', '[3/0] nothing | elapsed 1s | eta 1s'],
  ])('gives null for %s, so the page shows it as it is', (_name, line) => {
    expect(parseProgress(line)).toBeNull();
  });
});

describe('canDelete', () => {
  const run = { status: 'complete', current: false, protected: false, deleting: false } as const;

  it('lets an unprotected run that is neither shown nor going be deleted', () => {
    expect(canDelete(run)).toBe(true);
    expect(canDelete({ ...run, status: 'failed' })).toBe(true);
  });

  it.each([
    ['protected', { protected: true }],
    ['shown', { current: true }],
    ['going', { status: 'running' }],
    ['already deleting', { deleting: true }],
  ] as const)('hides delete on a %s run', (_name, change) => {
    expect(canDelete({ ...run, ...change })).toBe(false);
  });
});

describe('sizeText', () => {
  it.each([
    [4_000_000_000, '4.0 GB'],
    [350_000_000, '350.0 MB'],
    [12_500, '12.5 kB'],
  ])('writes %i bytes as %s: the largest unit under a thousand, one decimal', (bytes, want) => {
    expect(sizeText(bytes)).toBe(want);
  });

  it('writes nothing for a size not known yet, and 0 B for an empty run', () => {
    expect(sizeText(undefined)).toBe('');
    expect(sizeText(0)).toBe('0 B');
  });
});

describe('keepPolling', () => {
  const now = 1_000_000;

  it.each([
    ['a run going', 'running', false, 0, true],
    ['a run being deleted', 'complete', true, 0, true],
    ['only finished runs', 'complete', false, 0, false],
    ['finished runs while a started run is awaited', 'complete', false, now + 1, true],
    ['finished runs once the wait is over', 'complete', false, now - 1, false],
  ])('polls while a run is going, being deleted or awaited: %s', (_name, status, deleting, waitingUntil, want) => {
    expect(keepPolling([{ status, deleting }], waitingUntil, now)).toBe(want);
  });
});

describe('awaitStartedRun', () => {
  it('waits three minutes for a started run to appear', () => {
    expect(awaitStartedRun(1_000_000)).toBe(1_000_000 + 3 * 60 * 1000);
  });
});

const RUN = (id: number, over: Partial<{ status: 'running' | 'complete' | 'failed'; current: boolean; deleting: boolean }> = {}) => ({
  id,
  status: 'complete' as const,
  current: false,
  deleting: false,
  ...over,
});

describe('runs panel', () => {
  it.each([
    ['the one on the site', RUN(5, { current: true }), 'On the site'],
    ['one being deleted', RUN(4, { deleting: true }), 'Deleting…'],
    ['a failed one', RUN(3, { status: 'failed' }), 'Failed'],
    ['a complete one', RUN(2), 'Complete'],
    ['a running one', RUN(6, { status: 'running' }), 'Running'],
  ])('labels %s', (_, run, want) => {
    expect(runLabel(run)).toBe(want);
  });

  it.each([
    ['a later run publishes', 7, 'Publish'],
    ['an earlier run rolls back', 3, 'Roll back to'],
  ])('%s', (_, id, want) => {
    expect(publishVerb([RUN(5, { current: true }), RUN(7), RUN(3)], RUN(id))).toBe(want);
  });

  it.each([
    ['the latest few', false, [9, 8]],
    ['all when asked', true, [9, 8, 7]],
  ])('shows %s', (_, all, want) => {
    expect(shownRuns([RUN(9), RUN(8), RUN(7)], all, 2).map((r) => r.id)).toEqual(want);
  });

  it.each([
    ['one running', [RUN(2), RUN(3, { status: 'running' })], true],
    ['none running', [RUN(2)], false],
  ])('knows a run is going with %s', (_, runs, want) => {
    expect(runGoing(runs)).toBe(want);
  });
});

describe('runSize', () => {
  it.each([
    ['a finished run', 'complete', 4_000_000_000, '4.0 GB'],
    ['a failed run with nothing left', 'failed', 0, '0 B'],
    ['a run still going, its rows not yet recorded', 'running', 0, ''],
  ] as const)('writes %s', (_, status, bytes, want) => {
    expect(runSize(status, bytes)).toBe(want);
  });
});

describe('changesSummary', () => {
  const still = { run: 45, against: 44, new: 0, gone: 0, moved_1: 0, moved_10: 0, moved_50: 0, moved_100: 0, biggest: [] };

  it('says in one line that nothing changed when no rating or player did', () => {
    expect(changesSummary(still)).toEqual({ same: true, line: 'Run 45 rates everyone as run 44 did' });
  });

  it.each([
    ['a player gone', { ...still, gone: 1 }, []],
    ['a new player', { ...still, new: 1 }, []],
    ['a move under a point', { ...still, biggest: [{ change: 0.4 }] }, []],
    [
      'moves past some thresholds',
      { ...still, moved_1: 3, moved_10: 3, moved_50: 2, biggest: [{ change: -120 }] },
      [
        { points: 1, count: 3 },
        { points: 10, count: 3 },
        { points: 50, count: 2 },
      ],
    ],
  ])('lists only the thresholds someone crossed after %s', (_name, c, thresholds) => {
    expect(changesSummary(c)).toEqual({ same: false, thresholds });
  });
});
