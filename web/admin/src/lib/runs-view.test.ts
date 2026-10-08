import { describe, expect, it } from 'vitest';
import { awaitStartedRun, canDelete, keepPolling, parseProgress, sizeText } from './runs-view';

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
