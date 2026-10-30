import { describe, expect, it } from 'vitest';
import { cardView } from './tournament-card';

describe('cardView', () => {
  it("shows a tournament's card as admin wrote it", () => {
    expect(
      cardView({ icon: '🏆', tagline: 'Annual World Championship', summary: 'The pinnacle.', subtitle: '4-Player', cadence: 'Annual' }),
    ).toEqual({
      icon: '🏆',
      tagline: 'Annual World Championship',
      summary: 'The pinnacle.',
      subtitle: '4-Player',
      cadence: 'Annual',
    });
  });

  it('gives every card an icon, and leaves out what is not written', () => {
    const unset = { icon: '📜', tagline: null, summary: null, subtitle: null, cadence: null };
    expect(cardView({ icon: '', tagline: '', summary: '', subtitle: '', cadence: '' })).toEqual(unset);
    expect(cardView(undefined)).toEqual(unset);
  });
});
