import { describe, expect, it } from 'vitest';
import { matchRank } from './match';

describe('matchRank', () => {
  it.each([
    ['the name itself, in any case', 'Europe Central', 'europe central', 0],
    ['an underscore read as a space', 'America/New_York', 'america/new york', 0],
    ['the start of the name', 'Europe Central', 'eur', 1],
    ['inside the name', 'East Asia', 'asia', 2],
    ['each word typed starting a word of the name, in order', 'Europe Central', 'eu c', 3],
    ['words split at a slash', 'Asia/Shanghai', 'a s', 3],
    ['words out of order', 'US West', 'west us', null],
    ['nothing in common', 'US West', 'mars', null],
  ])('%s', (_, name, typed, want) => {
    expect(matchRank(name, typed)).toBe(want);
  });
});
