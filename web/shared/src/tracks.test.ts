import { describe, expect, it } from 'vitest';
import { trackName, trackShort } from './tracks';

describe('trackName', () => {
  it.each([
    ['community', 'Standard'],
    ['superblitz', 'SuperBlitz'],
    ['community_superblitz', 'Standard + SuperBlitz'],
    ['pulse', 'Pulse'],
    ['other', 'other'],
  ])('names %s', (slug, want) => {
    expect(trackName(slug)).toBe(want);
  });
});

describe('trackShort', () => {
  it.each([
    ['community', 'Std'],
    ['superblitz', 'SB'],
    ['community_superblitz', 'Std + SB'],
    ['pulse', 'Pulse'],
    ['other', 'other'],
  ])('shortens %s', (slug, want) => {
    expect(trackShort(slug)).toBe(want);
  });
});
