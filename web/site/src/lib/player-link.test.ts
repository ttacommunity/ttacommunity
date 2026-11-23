import { describe, expect, it } from 'vitest';
import { playerLinkClass } from './player-link';

describe('playerLinkClass', () => {
  it.each([
    { name: 'a name is the one name look', look: {}, want: 'player-link' },
    { name: 'first place in gold', look: { place: 1 as const }, want: 'player-link player-place-1' },
    { name: 'second place in silver', look: { place: 2 as const }, want: 'player-link player-place-2' },
    { name: 'third place in bronze', look: { place: 3 as const }, want: 'player-link player-place-3' },
    { name: 'last place set back', look: { place: 'last' as const }, want: 'player-link player-place-last' },
    { name: 'a name that is not the subject', look: { quiet: true }, want: 'player-link player-quiet' },
  ])('$name', ({ look, want }) => {
    expect(playerLinkClass(look)).toBe(want);
  });
});
