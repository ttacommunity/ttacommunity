// Fail when a built site's HTML carries inline code its CSP would block: a
// style attribute, or a <script> or <style> whose sha256 is not in _headers.
// Usage: node checks/csp-inline.mjs <site dist directory>
import { createHash } from 'node:crypto';
import { readFileSync, readdirSync } from 'node:fs';
import { join, relative } from 'node:path';

const dist = process.argv[2];
if (!dist) {
  console.error('usage: node checks/csp-inline.mjs <site dist directory>');
  process.exit(64);
}

const headers = readFileSync(join(dist, '_headers'), 'utf8');
const directive = (name) => headers.match(new RegExp(`${name} ([^;\\n]*)`))?.[1].split(/\s+/);
const allowed = { script: directive('script-src'), style: directive('style-src') };
if (!allowed.script || !allowed.style) {
  console.error(`csp-inline: ${dist}/_headers must set script-src and style-src`);
  process.exit(1);
}
const hashOf = (body) => `'sha256-${createHash('sha256').update(body, 'utf8').digest('base64')}'`;

const pages = readdirSync(dist, { recursive: true, encoding: 'utf8' }).filter((f) => f.endsWith('.html'));
if (pages.length === 0) {
  console.error(`csp-inline: no HTML in ${dist}`);
  process.exit(1);
}

let failures = 0;
const fail = (msg) => {
  console.error(`  ${msg}`);
  failures++;
};
for (const page of pages) {
  const html = readFileSync(join(dist, page), 'utf8');
  const where = relative('.', join(dist, page));
  if (/<[a-z][^>]*\sstyle=/i.test(html)) fail(`${where}: inline style attribute`);
  for (const [, tag, attrs, body] of html.matchAll(/<(script|style)(\s[^>]*)?>([\s\S]*?)<\/\1>/g)) {
    if (tag === 'script' && /\ssrc=/.test(attrs ?? '')) continue;
    const hash = hashOf(body);
    if (!allowed[tag].includes(hash)) fail(`${where}: inline <${tag}> not in ${tag}-src; add ${hash}`);
  }
}

if (failures > 0) {
  console.error(`csp-inline: ${failures} inline item(s) the CSP would block in ${dist}`);
  process.exit(1);
}
console.log(`  csp-inline: ${pages.length} page(s) in ${dist}, every inline script and style allowed by hash`);
