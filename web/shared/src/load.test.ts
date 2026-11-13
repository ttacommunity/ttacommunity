import { describe, expect, it } from 'vitest';
import { errorText, failed, settled, shownLoad } from './load';
import { RatingsApiError } from './ratings';

describe('errorText', () => {
  it.each([
    ['an error', new Error('the API is down'), 'the API is down'],
    ['a string', 'timed out', 'timed out'],
    ['anything else', 404, '404'],
  ])('gives %s as its message', (_, reason, want) => {
    expect(errorText(reason)).toBe(want);
  });
});

describe('failed', () => {
  it.each([
    ['a 404 from the API', new RatingsApiError(404, 'not found'), { kind: 'missing' }],
    ['another API status', new RatingsApiError(500, 'the API is down'), { kind: 'error', message: 'the API is down' }],
    ['a network failure', new TypeError('Failed to fetch'), { kind: 'error', message: 'Failed to fetch' }],
  ])('gives %s', (_, reason, want) => {
    expect(failed(reason)).toEqual(want);
  });
});

describe('settled', () => {
  it('gives an answer as ready', async () => {
    expect(await settled(Promise.resolve([1, 2]))).toEqual({ kind: 'ready', data: [1, 2] });
  });

  it.each([
    ['an error', new Error('the API is down'), 'the API is down'],
    ['anything else', 'timed out', 'timed out'],
  ])('gives a failure with %s as an error with its message', async (_, reason, message) => {
    expect(await settled(Promise.reject(reason))).toEqual({ kind: 'error', message });
  });
});

describe('shownLoad', () => {
  const ready = { kind: 'ready' as const, data: 1 };
  const loading = { kind: 'loading' as const };
  const failed = { kind: 'error' as const, message: 'down' };
  it.each([
    ['the last data while loading again', ready, loading, ready],
    ['loading the first time', null, loading, loading],
    ['new data once it is here', ready, { kind: 'ready' as const, data: 2 }, { kind: 'ready', data: 2 }],
    ['a failure, not old data', ready, failed, failed],
  ])('shows %s', (_name, last, next, want) => {
    expect(shownLoad(last, next)).toEqual(want);
  });
});
