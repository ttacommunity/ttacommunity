import { describe, expect, it } from 'vitest';
import { errorText, failed, settled } from './load';
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
