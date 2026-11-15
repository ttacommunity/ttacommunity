import { describe, expect, it, vi } from 'vitest';
import { askToPlay, fetchMyNostradamus, sendAnswers } from './nostradamus-me';

const answering = (status: number, body?: unknown) =>
  vi.fn(async () => new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }));

describe('fetchMyNostradamus', () => {
  it('reads the place with the session', async () => {
    const ok = answering(200, { status: 'accepted', answers: [{ question: 6, answer: ['Ann'], tokens: 1 }] });
    expect(await fetchMyNostradamus('https://api.example/', 'wc-2031', ok as unknown as typeof fetch)).toEqual({
      kind: 'signed-in',
      mine: { status: 'accepted', answers: [{ question: 6, answer: ['Ann'], tokens: 1 }] },
    });
    expect(ok).toHaveBeenCalledWith('https://api.example/v1/me/nostradamus/wc-2031', { credentials: 'include' });
  });
  it.each([
    ['signed out', 401, { kind: 'signed-out' }],
    ['not linked or banned', 403, { kind: 'not-linked' }],
  ])('is %s', async (_name, status, want) => {
    expect(await fetchMyNostradamus('https://api.example', 'wc-2031', answering(status, { error: 'no' }) as unknown as typeof fetch)).toEqual(want);
  });
});

describe('player changes', () => {
  it.each([
    ['asking to play', (f: typeof fetch) => askToPlay('https://api.example', 'wc-2031', f), 'POST', '/v1/me/nostradamus/wc-2031/entry'],
    ['answers', (f: typeof fetch) => sendAnswers('https://api.example', 'wc-2031', 4, [{ question: 6, answer: ['Ann'], tokens: 0 }], f), 'PUT', '/v1/me/nostradamus/wc-2031/rounds/4/answers'],
  ])('sends %s with the session and answers a refusal with its message', async (_name, send, method, path) => {
    const ok = answering(200, {});
    expect(await send(ok as unknown as typeof fetch)).toEqual({ ok: true });
    expect(ok).toHaveBeenCalledWith(`https://api.example${path}`, expect.objectContaining({ method, credentials: 'include' }));
    expect(await send(answering(409, { error: "the round's deadline has passed" }) as unknown as typeof fetch)).toEqual({ ok: false, message: "the round's deadline has passed" });
  });
});
