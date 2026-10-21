/** Signing in with an emailed link: asking for one behind Turnstile, and the page the link opens, which posts its token to the api when the player asks. */
import { change, failureMessage, type Done } from './account';

/** TURNSTILE_SCRIPT is Turnstile's script, loaded from Cloudflare as it must be, to render the widget when the form asks. */
export const TURNSTILE_SCRIPT = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';

/** TURNSTILE_ACTION names the form to Turnstile; the api checks the token was solved for it. */
export const TURNSTILE_ACTION = 'email-signin';

/** sendEmailLink asks the api to email address a sign-in link, which comes back to back. */
export function sendEmailLink(base: string, address: string, turnstile: string, back: string, fetchFn: typeof fetch = fetch): Promise<Done> {
  return change(fetchFn, base, 'POST', '/v1/auth/email', { email: address.trim(), turnstile, return: back });
}

/** tokenFromHash reads the link's token from the page's fragment, or null when it holds none. */
export function tokenFromHash(hash: string): string | null {
  const token = hash.replace(/^#/, '');
  return /^[A-Z2-7]{52}$/.test(token) ? token : null;
}

/** Outcome is what opening a link did, as the api says. */
export type Outcome = 'first' | 'ok' | 'added';

/** Finished is how opening a link went: signed in or added, the link expired or used, or refused with the api's message. */
export type Finished = { kind: 'done'; outcome: Outcome; back: string } | { kind: 'expired'; message: string } | { kind: 'refused'; message: string };

/** finishEmailLink posts the link's token to the api, which signs this browser in. */
export async function finishEmailLink(base: string, token: string, fetchFn: typeof fetch = fetch): Promise<Finished> {
  let res: Response;
  try {
    res = await fetchFn(`${base.replace(/\/+$/, '')}/v1/auth/email/finish`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    });
  } catch {
    return { kind: 'refused', message: 'The site could not reach its server. Check your connection and try again.' };
  }
  if (res.ok) {
    const body = (await res.json()) as { outcome: Outcome; back: string };
    return { kind: 'done', outcome: body.outcome, back: body.back };
  }
  const message = await failureMessage(res);
  return res.status === 410 ? { kind: 'expired', message } : { kind: 'refused', message };
}

/** emailFinishTarget is where the browser goes once signed in: back where it was, a first sign-in to the account page with the way back, an address added to the account page. */
export function emailFinishTarget(outcome: Outcome, back: string): string {
  const path = /^\/(?!\/)[\x21-\x7e]*$/.test(back) && !/[\\#]/.test(back) ? back : '/account/';
  if (outcome === 'added') return '/account/?signin=email-added';
  if (outcome === 'first') {
    const q = new URLSearchParams();
    if (path !== '/account/') q.set('back', path);
    q.set('signin', 'first');
    return `/account/?${q}`;
  }
  return `${path}${path.includes('?') ? '&' : '?'}signin=ok`;
}
