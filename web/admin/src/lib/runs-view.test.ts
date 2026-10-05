import { describe, expect, it } from 'vitest';
import { canDelete, keepPolling, parseProgress, sizeText } from './runs-view';

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

  it('gives null for a line that is not a step, so the page shows it as it is', () => {
    expect(parseProgress('')).toBeNull();
    expect(parseProgress('run 33: community, engine 218f9a35')).toBeNull();
    expect(parseProgress('[3/0] nothing | elapsed 1s | eta 1s')).toBeNull();
  });
});

describe('canDelete', () => {
  const run = { status: 'complete', current: false, protected: false, deleting: false } as const;

  it('lets an unprotected run that is neither shown nor going be deleted', () => {
    expect(canDelete(run)).toBe(true);
    expect(canDelete({ ...run, status: 'failed' })).toBe(true);
  });

  it('hides delete on a protected, shown, going or already deleting run', () => {
    expect(canDelete({ ...run, protected: true })).toBe(false);
    expect(canDelete({ ...run, current: true })).toBe(false);
    expect(canDelete({ ...run, status: 'running' })).toBe(false);
    expect(canDelete({ ...run, deleting: true })).toBe(false);
  });
});

describe('sizeText', () => {
  it('writes a size in the largest unit under a thousand, one decimal', () => {
    expect(sizeText(4_000_000_000)).toBe('4.0 GB');
    expect(sizeText(350_000_000)).toBe('350.0 MB');
    expect(sizeText(12_500)).toBe('12.5 kB');
  });

  it('writes nothing for a size not known yet, and 0 B for an empty run', () => {
    expect(sizeText(undefined)).toBe('');
    expect(sizeText(0)).toBe('0 B');
  });
});

describe('keepPolling', () => {
  it('polls while a run is going or being deleted', () => {
    expect(keepPolling([{ status: 'running', deleting: false }])).toBe(true);
    expect(keepPolling([{ status: 'complete', deleting: true }])).toBe(true);
    expect(keepPolling([{ status: 'complete', deleting: false }])).toBe(false);
  });
});
