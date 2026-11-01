import { describe, expect, it } from 'vitest';
import { EVENTS, endpointChanged, toggled } from './notification-endpoints';

const SAVED = { name: 'Admins', url: 'https://discord.com/api/webhooks/100000000000000001/token', events: ['link_request' as const], prefix: '' };

describe('notification endpoints', () => {
  it('offers every event the API knows', () => {
    expect(EVENTS.map((e) => e.key)).toEqual(['link_request', 'blitz_report', 'blitz_flag']);
  });

  it.each([
    ['unchanged', SAVED, false],
    ['renamed', { ...SAVED, name: 'Admin channel' }, true],
    ['given a new URL', { ...SAVED, url: 'https://discord.com/api/webhooks/100000000000000002/other' }, true],
    ['given a prefix', { ...SAVED, prefix: '[dev]' }, true],
    ['taking another event', { ...SAVED, events: ['link_request' as const, 'blitz_flag' as const] }, true],
    ['taking the same events in another order', { ...SAVED, events: [...SAVED.events] }, false],
    ['left without a name', { ...SAVED, name: '  ' }, false],
    ['left without a URL', { ...SAVED, url: '' }, false],
  ])('can be saved when %s: %s', (_, draft, want) => {
    expect(endpointChanged(SAVED, draft)).toBe(want);
  });

  it.each([
    ['adding an event', ['blitz_flag' as const], 'link_request' as const, true, ['link_request', 'blitz_flag']],
    ['adding one already there', ['blitz_flag' as const], 'blitz_flag' as const, true, ['blitz_flag']],
    ['taking an event out', ['link_request' as const, 'blitz_flag' as const], 'link_request' as const, false, ['blitz_flag']],
  ])('toggles events: %s', (_, events, event, on, want) => {
    expect(toggled(events, event, on)).toEqual(want);
  });
});
