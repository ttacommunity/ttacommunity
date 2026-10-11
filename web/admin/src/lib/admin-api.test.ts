import { describe, expect, it, vi } from 'vitest';
import { adminDownload, adminRequest, exportFileName, firstError, type Load } from './admin-api';

function answer(status: number, body: unknown) {
  return vi.fn(async () => new Response(body === null ? null : JSON.stringify(body), { status }));
}

describe('adminRequest', () => {
  it('returns the data of a good answer, and nothing for 204', async () => {
    expect(await adminRequest('/api/me', {}, answer(200, { name: 'Owner', role: 'admin' }))).toEqual({
      ok: true,
      data: { name: 'Owner', role: 'admin' },
    });
    expect(await adminRequest('/api/tournaments/worlds', { method: 'PATCH' }, answer(204, null))).toEqual({ ok: true, data: null });
  });

  it('says plainly why a request was refused', async () => {
    expect(await adminRequest('/api/me', {}, answer(401, { error: 'x' }))).toEqual({
      ok: false,
      status: 401,
      message: 'Your sign-in has ended. Reload the page to sign in again.',
    });
    expect(await adminRequest('/api/me', {}, answer(403, { error: 'not an admin or organiser' }))).toEqual({
      ok: false,
      status: 403,
      message: 'Refused: not an admin or organiser.',
    });
    expect(await adminRequest('/api/runs', { method: 'POST' }, answer(409, { error: 'a rating run is already going' }))).toEqual({
      ok: false,
      status: 409,
      message: 'Refused: a rating run is already going.',
    });
    expect(await adminRequest('/api/x', {}, answer(500, 'not json'))).toEqual({
      ok: false,
      status: 500,
      message: 'The API failed (500). Try again, or tell an admin.',
    });
  });

  it('sends JSON with the method and body it is given, from this site', async () => {
    const f = answer(204, null);
    await adminRequest('/api/tournaments/worlds', { method: 'PATCH', json: { community: true } }, f);
    expect(f).toHaveBeenCalledWith('/api/tournaments/worlds', {
      method: 'PATCH',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: '{"community":true}',
    });
  });
});

describe('adminDownload', () => {
  it('gives the file of a good answer', async () => {
    const f = vi.fn(async () => new Response('PK zip', { status: 200, headers: { 'Content-Type': 'application/zip' } }));
    const r = await adminDownload('/api/export', f);
    expect(r.ok).toBe(true);
    expect(r.ok && (await r.data.text())).toBe('PK zip');
    expect(f).toHaveBeenCalledWith('/api/export', { method: 'GET', credentials: 'same-origin' });
  });

  it('says plainly why a download was refused', async () => {
    expect(await adminDownload('/api/export', answer(401, { error: 'x' }))).toMatchObject({ ok: false, status: 401 });
    expect(await adminDownload('/api/export', answer(403, { error: 'not allowed here' }))).toEqual({
      ok: false,
      status: 403,
      message: 'Refused: not allowed here.',
    });
  });

  it('says a download that broke off part way should be tried again', async () => {
    const body = new ReadableStream({
      start(c) {
        c.enqueue(new TextEncoder().encode('PK'));
        c.error(new TypeError('network error'));
      },
    });

    const r = await adminDownload('/api/export', vi.fn(async () => new Response(body, { status: 200 })));

    expect(r).toMatchObject({ ok: false, message: expect.stringContaining('broke off') });
  });
});

describe('a server error', () => {
  it.each([
    ['a request', (f: ReturnType<typeof answer>) => adminRequest('/api/me', {}, f)],
    ['a download', (f: ReturnType<typeof answer>) => adminDownload('/api/export', f)],
  ])('quotes its request id, to report it by, for %s', async (_, call) => {
    const r = await call(answer(500, { error: 'internal error', request_id: '0123456789abcdef' }));

    expect(r).toMatchObject({ ok: false, status: 500, message: expect.stringContaining('0123456789abcdef') });
  });
});

describe('firstError', () => {
  const ready: Load<number> = { kind: 'ready', data: 1 };
  const loading: Load<number> = { kind: 'loading' };
  const failed = (message: string): Load<number> => ({ kind: 'error', message });

  it.each([
    ['nothing failed', [ready, loading], null],
    ['one failed', [ready, failed('no tournament')], 'no tournament'],
    ['both failed: the first', [failed('sign-in ended'), failed('no tournament')], 'sign-in ended'],
  ])('%s', (_, loads, want) => {
    expect(firstError(...loads)).toBe(want);
  });
});

describe('exportFileName', () => {
  it("names a download by the day it was taken, so several don't overwrite each other", () => {
    expect(exportFileName(new Date(2026, 9, 4, 22, 5))).toBe('ttacommunity-data-2026-10-04.zip');
  });
});
