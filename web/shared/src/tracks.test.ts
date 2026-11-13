import { describe, expect, it } from 'vitest';
import { trackName } from './tracks';

describe('trackName', () => {
  it.each([
    ['community', 'Standard'],
    ['superblitz', 'SuperBlitz'],
    ['community_superblitz', 'Standard + SuperBlitz'],
    ['pulse', 'TTA Pulse'],
    ['other', 'other'],
  ])('names %s', (slug, want) => {
    expect(trackName(slug)).toBe(want);
  });
});
