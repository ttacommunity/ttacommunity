import { describe, expect, it } from 'vitest';
import { emailFinishTarget, finishEmailLink, sendEmailLink, tokenFromHash, TURNSTILE_SCRIPT } from './email-sign-in';

/** recorded is a fetch answering status with body, keeping what it was asked. */
function recorded(status: number, body: unknown) {
  const calls: { url: string; init?: RequestInit }[] = [];
  const fetchFn = ((url: string, init?: RequestInit) => {
    calls.push({ url, init });
    return Promise.resolve(new Response(body === null ? null : JSON.stringify(body), { status }));
  }) as typeof fetch;
  return { calls, fetchFn };
}

const token = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567ABCDEFGHIJKLMNOPQRST';

describe('tokenFromHash', () => {
  it.each([
    ['a link token', `#${token}`, token],
    ['no fragment', '', null],
    ['too short', '#ABC', null],
    ['not the api alphabet', `#${token.toLowerCase()}`, null],
  ])('%s', (_name, hash, want) => {
    expect(tokenFromHash(hash)).toBe(want);
  });
});

describe('emailFinishTarget', () => {
  it.each([
    ['a returning player, back where they were', 'ok', '/ratings/', '/ratings/?signin=ok'],
    ['a returning player, keeping the query', 'ok', '/ratings/player/?name=ann', '/ratings/player/?name=ann&signin=ok'],
    ['a first sign-in, welcomed with the way back', 'first', '/ratings/', '/account/?back=%2Fratings%2F&signin=first'],
    ['a first sign-in from the account page', 'first', '/account/', '/account/?signin=first'],
    ['an address added', 'added', '/account/', '/account/?signin=email-added'],
    ['somewhere not on the site', 'ok', '//evil.example/', '/account/?signin=ok'],
  ] as const)('%s', (_name, outcome, back, want) => {
    expect(emailFinishTarget(outcome, back)).toBe(want);
  });
});

describe('sendEmailLink', () => {
  it('sends the address, the Turnstile token and the way back, with credentials', async () => {
    const { calls, fetchFn } = recorded(204, null);
    expect(await sendEmailLink('https://api.example', ' player@example.test ', 'tok', '/blitz/', fetchFn)).toEqual({ ok: true });
    expect(calls[0].url).toBe('https://api.example/v1/auth/email');
    expect(calls[0].init?.method).toBe('POST');
    expect(calls[0].init?.credentials).toBe('include');
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ email: 'player@example.test', turnstile: 'tok', return: '/blitz/' });
  });

  it("returns the api's refusal", async () => {
    expect(await sendEmailLink('https://api.example', 'player@example.test', 'tok', '/', recorded(429, { error: 'too many sign-in links sent: try again later' }).fetchFn)).toEqual({
      ok: false,
      message: 'too many sign-in links sent: try again later',
    });
  });
});

describe('finishEmailLink', () => {
  it('posts the token and reads where to go', async () => {
    const { calls, fetchFn } = recorded(200, { outcome: 'first', back: '/blitz/' });
    expect(await finishEmailLink('https://api.example', token, fetchFn)).toEqual({ kind: 'done', outcome: 'first', back: '/blitz/' });
    expect(calls[0].url).toBe('https://api.example/v1/auth/email/finish');
    expect(calls[0].init?.credentials).toBe('include');
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ token });
  });

  it.each([
    ['an expired or used link', 410, { error: 'the link has expired or has been used' }, 'expired'],
    ['an address with an account of its own', 409, { error: 'that email address has an account of its own here' }, 'refused'],
  ])('%s', async (_name, status, body, kind) => {
    expect((await finishEmailLink('https://api.example', token, recorded(status, body).fetchFn)).kind).toBe(kind);
  });

  it('says so when the api cannot be reached', async () => {
    const unreachable = (() => Promise.reject(new TypeError('Failed to fetch'))) as typeof fetch;
    expect((await finishEmailLink('https://api.example', token, unreachable)).kind).toBe('refused');
  });
});

describe('TURNSTILE_SCRIPT', () => {
  it("is Cloudflare's own, rendered explicitly: never a copy", () => {
    expect(TURNSTILE_SCRIPT).toBe('https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit');
  });
});
