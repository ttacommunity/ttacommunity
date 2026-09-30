// Fits the built site's _headers to where it is deployed: the API it reads,
// and whether search engines may index it.

/**
 * siteHeaders returns `headers` (a _headers file) with the CSP's connect-src
 * set to `'self'` and the origin of `apiUrl`, plus `X-Robots-Tag: noindex`
 * when `noindex` is set. It throws on a missing connect-src or a non-http URL.
 */
export function siteHeaders(headers: string, apiUrl: string, noindex: boolean): string {
  const api = new URL(apiUrl);
  if (api.protocol !== 'https:' && api.protocol !== 'http:') {
    throw new Error(`PUBLIC_API_URL must be http or https, not ${api.protocol}`);
  }
  const connectSrc = /connect-src [^;\n]*/;
  if (!connectSrc.test(headers)) throw new Error('_headers sets no connect-src to point at the API');
  const out = headers.replace(connectSrc, `connect-src 'self' ${api.origin}`);
  if (!noindex) return out;
  const everyPath = /^\/\*\n/m;
  if (!everyPath.test(out)) throw new Error('_headers has no /* block to add noindex to');
  return out.replace(everyPath, '/*\n  X-Robots-Tag: noindex\n');
}
