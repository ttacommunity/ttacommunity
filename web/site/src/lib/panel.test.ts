import { describe, expect, it } from 'vitest';
import { panelClass } from './panel';

describe('panelClass', () => {
  it.each([
    { name: 'a plain panel is the card', look: {}, want: 'rating-card panel-card' },
    { name: 'commentary takes a gold stripe', look: { stripe: 'gold' as const }, want: 'rating-card panel-card panel-stripe panel-stripe-gold' },
    { name: 'a finding takes a green stripe', look: { stripe: 'green' as const }, want: 'rating-card panel-card panel-stripe panel-stripe-green' },
    { name: 'a deep dive takes a bronze stripe', look: { stripe: 'bronze' as const }, want: 'rating-card panel-card panel-stripe panel-stripe-bronze' },
    { name: 'a picture behind the panel', look: { art: '/img/groups/classic.jpg' }, want: 'rating-card panel-card panel-art' },
    { name: 'every setting at once', look: { stripe: 'gold' as const, art: '/a.jpg' }, want: 'rating-card panel-card panel-stripe panel-stripe-gold panel-art' },
    { name: 'an empty picture is no picture', look: { art: '' }, want: 'rating-card panel-card' },
    { name: 'a form or prose takes the reading width', look: { size: 'reading' as const }, want: 'rating-card panel-card panel-reading' },
    { name: 'full width is the default', look: { size: 'full' as const }, want: 'rating-card panel-card' },
    { name: 'a panel of text to read takes the reading look', look: { holds: 'text' as const }, want: 'rating-card panel-card panel-text' },
    { name: 'a table with many columns takes the screen\'s width', look: { size: 'screen' as const }, want: 'rating-card panel-card panel-screen' },
  ])('$name', ({ look, want }) => {
    expect(panelClass(look)).toBe(want);
  });
});
