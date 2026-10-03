import { describe, expect, it, vi } from 'vitest';
import { adminRequest } from './admin-api';

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
