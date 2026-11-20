import { describe, expect, it, vi } from 'vitest';
import { closesLine, countLine, fetchMySignUp, signUp, signUpBox, withdraw, type MySignUp, type SignUps } from './sign-up';

const taking: SignUps = { open: true, closes_at: null, taking: true, waiting: 0 };
const closed: SignUps = { open: false, closes_at: null, taking: false, waiting: 0 };
const mine = (status: MySignUp['status'], linked: boolean, name: string | null = null): MySignUp => ({ status, linked, name });

describe('signUpBox', () => {
  it.each([
    ['closed, a visitor', closed, null, { kind: 'hidden' }],
    ['closed, an entrant', closed, mine('entered', true), { kind: 'hidden' }],
    ['closed, not signed up', closed, mine('none', false), { kind: 'hidden' }],
    ['closed, waiting for a link', closed, mine('pending', false, 'Newcomer'), { kind: 'pending', name: 'Newcomer', canWithdraw: false }],
    ['taking, still asking', taking, undefined, { kind: 'checking' }],
    ['taking, a visitor', taking, null, { kind: 'sign-in' }],
    ['taking, linked', taking, mine('none', true), { kind: 'enter' }],
    ['taking, unlinked', taking, mine('none', false), { kind: 'give-name' }],
    ['taking, an entrant', taking, mine('entered', true), { kind: 'entered' }],
    ['taking, waiting for a link', taking, mine('pending', false, 'Newcomer'), { kind: 'pending', name: 'Newcomer', canWithdraw: true }],
  ])('%s', (_name, s, m, want) => {
    expect(signUpBox(s, m)).toEqual(want);
  });
});

describe('countLine', () => {
  it.each([
    [0, 0, 'No one has signed up yet.'],
    [1, 0, '1 player entered.'],
    [12, 0, '12 players entered.'],
    [12, 1, '12 players entered, and 1 waiting for their account to be linked.'],
    [0, 3, 'No one entered yet, and 3 waiting for their account to be linked.'],
  ])('%i entered, %i waiting', (entered, waiting, want) => {
    expect(countLine(entered, waiting)).toBe(want);
  });
});

describe('closesLine', () => {
  it.each([
    ['a closing time', { ...taking, closes_at: '2026-12-01T07:00:00Z' }, 'Sign-ups close on 1 Dec 2026, 18:00 (Australia/Sydney), or when round 1 is paired.'],
    ['no closing time', taking, 'Sign-ups close when round 1 is paired.'],
  ])('%s', (_name, s, want) => {
    expect(closesLine(s, 'Australia/Sydney')).toBe(want);
  });
});

describe('requests', () => {
  const answering = (status: number, body: unknown = null) => vi.fn(async () => new Response(body === null ? null : JSON.stringify(body), { status }));
  it('reads the sign-up with the session, null when signed out', async () => {
    const ok = answering(200, mine('entered', true));
    expect(await fetchMySignUp('https://api.example/', 'us_open', '2027', ok)).toEqual(mine('entered', true));
    expect(ok).toHaveBeenCalledWith('https://api.example/v1/me/sign-ups/us_open/2027', { credentials: 'include' });
    expect(await fetchMySignUp('https://api.example', 'us_open', '2027', answering(401, { error: 'not signed in' }))).toBeNull();
  });
  it.each([
    ['signing up linked', (f: typeof fetch) => signUp('https://api.example', 'us_open', '2027', null, f), 'PUT', '{}'],
    ['signing up with a username', (f: typeof fetch) => signUp('https://api.example', 'us_open', '2027', '  Newcomer ', f), 'PUT', '{"tta_name":"Newcomer"}'],
  ])('sends %s with the session', async (_name, send, method, body) => {
    const ok = answering(200, mine('pending', false, 'Newcomer'));
    expect(await send(ok as unknown as typeof fetch)).toEqual({ ok: true });
    expect(ok).toHaveBeenCalledWith('https://api.example/v1/me/sign-ups/us_open/2027', expect.objectContaining({ method, credentials: 'include', body }));
  });
  it('withdraws with the session, and answers a refusal with its message', async () => {
    const ok = answering(204);
    expect(await withdraw('https://api.example', 'us_open', '2027', ok as unknown as typeof fetch)).toEqual({ ok: true });
    expect(ok).toHaveBeenCalledWith('https://api.example/v1/me/sign-ups/us_open/2027', expect.objectContaining({ method: 'DELETE', credentials: 'include' }));
    const refused = answering(409, { error: 'sign-ups are closed' });
    expect(await withdraw('https://api.example', 'us_open', '2027', refused as unknown as typeof fetch)).toEqual({ ok: false, message: 'sign-ups are closed' });
  });
});
