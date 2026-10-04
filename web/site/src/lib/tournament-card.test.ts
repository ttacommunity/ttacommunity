import { describe, expect, it } from 'vitest';
import { cardView, circuitCard } from './tournament-card';

describe('cardView', () => {
  it("shows a tournament's card as admin wrote it", () => {
    expect(cardView({ icon: '🏆', tagline: 'Annual World Championship', summary: 'The pinnacle.' })).toEqual({
      icon: '🏆',
      tagline: 'Annual World Championship',
      summary: 'The pinnacle.',
    });
  });

  it('gives every card an icon, and leaves out what is not written', () => {
    expect(cardView({ icon: '', tagline: '', summary: '' })).toEqual({ icon: '📜', tagline: null, summary: null });
    expect(cardView(undefined)).toEqual({ icon: '📜', tagline: null, summary: null });
  });
});

describe('circuitCard', () => {
  it("shows a circuit's own description as its card", () => {
    expect(circuitCard({ icon: '🎾', formatBadge: 'Major Opens', summary: 'Every slam.' })).toEqual({
      icon: '🎾',
      tagline: 'Major Opens',
      summary: 'Every slam.',
    });
  });
});
