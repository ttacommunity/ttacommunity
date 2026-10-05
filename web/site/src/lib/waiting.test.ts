import { describe, expect, it } from 'vitest';
import { waitingText } from './waiting';

describe('waitingText', () => {
  it('says what is loading', () => {
    expect(waitingText({ kind: 'loading' }, 'this player')).toBe('Loading this player…');
  });

  it('says what could not load, and why', () => {
    expect(waitingText({ kind: 'error', message: 'unavailable' }, 'this player')).toBe('Could not load this player: unavailable');
  });

  it('says nothing once loaded', () => {
    expect(waitingText({ kind: 'ready' }, 'this player')).toBeNull();
  });
});
