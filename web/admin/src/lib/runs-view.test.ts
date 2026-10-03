import { describe, expect, it } from 'vitest';
import { parseProgress } from './runs-view';

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
