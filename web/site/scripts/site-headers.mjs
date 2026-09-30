// Run after astro build: points dist/_headers' connect-src at PUBLIC_API_URL,
// and adds noindex when SITE_NOINDEX=1 (preview builds).
// Usage: node scripts/site-headers.mjs <dist directory>
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { siteHeaders } from '../src/lib/site-headers.ts';

const dist = process.argv[2];
if (!dist) {
  console.error('usage: node scripts/site-headers.mjs <dist directory>');
  process.exit(64);
}
// The same default as PUBLIC_API_URL's in astro.config.mjs.
const apiUrl = process.env.PUBLIC_API_URL || 'http://127.0.0.1:8080';
const noindex = process.env.SITE_NOINDEX === '1';

const file = join(dist, '_headers');
writeFileSync(file, siteHeaders(readFileSync(file, 'utf8'), apiUrl, noindex));
console.log(`site-headers: connect-src ${new URL(apiUrl).origin}${noindex ? ', noindex' : ''}`);
