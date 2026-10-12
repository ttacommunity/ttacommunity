// The site's committed fonts (public/fonts/): check Google for newer files,
// update our copies, or verify the committed ones. The build never fetches.
// Usage: node scripts/fonts.mjs check|update|verify
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { FONTS, changes, fontFile, googleCssUrl, parseGoogleCss, pick, renderCss } from '../src/lib/fonts.ts';

const DIR = fileURLToPath(new URL('../public/fonts/', import.meta.url));
const MANIFEST = join(DIR, 'manifest.json');
// Google serves woff2 only to browsers it recognises; this one is a current Chrome.
const UA = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36';

const sha256 = (buf) => createHash('sha256').update(buf).digest('hex');
const ours = () => (existsSync(MANIFEST) ? JSON.parse(readFileSync(MANIFEST, 'utf8')).faces : []);

async function googleFaces() {
  const res = await fetch(googleCssUrl(FONTS), { headers: { 'User-Agent': UA } });
  if (!res.ok) throw new Error(`Google Fonts answered ${res.status}`);
  return pick(parseGoogleCss(await res.text()), FONTS);
}

async function check() {
  const found = changes(ours(), await googleFaces());
  if (found.length === 0) {
    console.log(`fonts: up to date (${ours().length} faces)`);
    return 0;
  }
  console.log(`fonts: Google has newer fonts (${found.length} faces); run pnpm fonts:update and commit the result`);
  for (const c of found) console.log(`  ${c}`);
  return 1;
}

async function update() {
  const faces = await googleFaces();
  const files = new Map();
  for (const f of faces) {
    const file = fontFile(f);
    if (!files.has(file)) {
      const res = await fetch(f.url, { headers: { 'User-Agent': UA } });
      const buf = Buffer.from(await res.arrayBuffer());
      if (!res.ok || buf.subarray(0, 4).toString('latin1') !== 'wOF2') throw new Error(`${f.url}: not a woff2 file`);
      writeFileSync(join(DIR, file), buf);
      files.set(file, sha256(buf));
    }
  }
  const removed = readdirSync(DIR).filter((n) => n.endsWith('.woff2') && !files.has(n));
  for (const n of removed) rmSync(join(DIR, n));
  writeFileSync(join(DIR, 'fonts.css'), renderCss(faces));
  const manifest = { source: googleCssUrl(FONTS), faces: faces.map((f) => ({ ...f, file: fontFile(f), sha256: files.get(fontFile(f)) })) };
  writeFileSync(MANIFEST, `${JSON.stringify(manifest, null, 2)}\n`);
  console.log(`fonts: ${faces.length} faces in ${files.size} files written; ${removed.length} old files removed`);
  return 0;
}

function verify() {
  const bad = ours().filter((f) => !existsSync(join(DIR, f.file)) || sha256(readFileSync(join(DIR, f.file))) !== f.sha256);
  if (ours().length === 0 || bad.length > 0) {
    console.error(`fonts: ${ours().length === 0 ? 'no manifest' : [...new Set(bad.map((f) => f.file))].join(', ') + ' missing or changed'}: run pnpm fonts:update`);
    return 1;
  }
  console.log(`fonts: ${ours().length} faces verified`);
  return 0;
}

const modes = { check, update, verify };
const mode = modes[process.argv[2]];
if (!mode) {
  console.error('usage: node scripts/fonts.mjs check|update|verify');
  process.exit(64);
}
process.exit(await mode());
