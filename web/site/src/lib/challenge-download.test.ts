import { describe, expect, it } from 'vitest';
import { downloadUrls } from './challenge-download';

describe('downloadUrls', () => {
  it.each([
    ['the api', 'https://api.example.test'],
    ['the api with a trailing slash', 'https://api.example.test/'],
  ])('%s', (_name, api) => {
    expect(downloadUrls(api)).toEqual({
      csv: 'https://api.example.test/v1/challenges/entries.csv',
      json: 'https://api.example.test/v1/challenges/entries',
    });
  });
});
