// Warn, never fail, when a page's CSS changes the look of a shared piece: a rule whose styled element is a class the shared
// styles define, or a form control, setting more than layout. Prints each with its file and line for a person to decide on.
// Usage: node checks/shared-look.mjs [web root]   (default: web/)
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const root = process.argv[2] ?? 'web';

// Rules kept on purpose: "file|selector" and why. A kept rule that no longer exists is reported, to be removed here.
const KEPT = new Map([
  ['site/src/components/ViewBar.css|.view-bar', 'the bar component (ViewBar) sets its own padding'],
  ['site/src/components/HomeHeader.css|.page-header.home-hero', "the home header's own size, on PageHeader's card"],
  ['site/src/components/Bracket.css|button.bracket-name:hover', "the bracket's names are buttons drawn as names (one component)"],
  ['site/src/components/Bracket.css|button.bracket-name:focus-visible', "the bracket's names are buttons drawn as names (one component)"],
  ['site/src/components/GamesTable.css|.games-filters select', 'undoes the capitals the filters inherit, so the select looks as the shared field'],
  ['site/src/components/PlayerSearch.css|.search-input-wrapper .input-antique', "room for the search icon inside the field (PlayerSearch's own CSS)"],
  ['admin/src/components/AdminApp.css|.rating-card.admin-card', "admin's card (AdminCard) on the shared panel"],
  ['admin/src/components/AdminApp.css|.multi-pick > summary', "MultiPick's summary is drawn as a field, so it has no marker"],
]);

// Properties that place a piece rather than change how it looks.
const LAYOUT = new Set(['width', 'min-width', 'max-width', 'height', 'min-height', 'max-height', 'margin', 'margin-top', 'margin-bottom', 'margin-left', 'margin-right',
  'margin-inline', 'margin-block', 'flex', 'flex-grow', 'flex-shrink', 'flex-basis', 'flex-direction', 'flex-wrap', 'grid-area', 'grid-column', 'grid-row', 'order',
  'align-self', 'justify-self', 'align-items', 'justify-content', 'gap', 'position', 'top', 'left', 'right', 'bottom', 'inset', 'z-index', 'display', 'box-sizing',
  'container-type', 'cursor', 'white-space', 'overflow', 'overflow-x', 'overflow-y', 'text-align', 'vertical-align', 'transition', 'animation', 'content']);
const CONTROLS = new Set(['input', 'select', 'textarea', 'button', 'summary']);
// Words the shared styles use as state on many pieces, never a piece of their own.
const STATES = new Set(['active', 'open', 'current', 'win', 'loss', 'draw']);

/** files lists the files under dir with one of the extensions. */
function files(dir, exts) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === 'node_modules' || name === 'dist' ? [] : files(path, exts);
    return exts.some((e) => name.endsWith(e)) ? [path] : [];
  });
}

/** rules returns each rule's selector, declarations and line in css, inside @media and other blocks too. */
function rules(css) {
  const out = [];
  const text = css.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, ' '));
  const re = /([^{}]*)\{([^{}]*)\}|@[^{]*\{|\}/g;
  for (const m of text.matchAll(re)) {
    if (m[1] === undefined || m[1].trim().startsWith('@')) continue;
    const decls = new Map(m[2].split(';').filter((d) => d.includes(':')).map((d) => [d.split(':')[0].trim(), d.slice(d.indexOf(':') + 1).trim()]));
    const line = text.slice(0, m.index + m[1].search(/\S/)).split('\n').length;
    out.push({ selector: m[1].trim().replace(/\s+/g, ' '), decls, line });
  }
  return out;
}

/** subject is the part of a selector naming the element it styles, without the arguments of :has(), :is(), :not() and :where(). */
function subject(selector) {
  const bare = selector.replace(/:(has|is|not|where)\((?:[^()]|\([^()]*\))*\)/g, '');
  return bare.split(/\s*[\s>+~]\s*/).filter(Boolean).at(-1) ?? '';
}

const shared = new Set();
for (const f of files(join(root, 'shared/styles/components'), ['.css'])) {
  for (const r of rules(readFileSync(f, 'utf8'))) for (const c of r.selector.matchAll(/\.([a-zA-Z][\w-]*)/g)) if (!STATES.has(c[1])) shared.add(c[1]);
}

const warnings = [];
const seen = new Set();
for (const f of [...files(join(root, 'site/src'), ['.css', '.astro']), ...files(join(root, 'admin/src'), ['.css', '.astro'])]) {
  let css = readFileSync(f, 'utf8');
  if (f.endsWith('.astro')) css = [...css.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
  const file = relative(root, f);
  for (const r of rules(css)) {
    const look = [...r.decls.keys()].filter((p) => !LAYOUT.has(p) && !p.startsWith('--'));
    if (look.length === 0) continue;
    for (const part of r.selector.split(',').map((s) => s.trim())) {
      const s = subject(part);
      const why = [...s.matchAll(/\.([a-zA-Z][\w-]*)/g)].map((c) => c[1]).filter((c) => shared.has(c));
      const tag = /^[a-z]+/.exec(s)?.[0];
      if (tag && CONTROLS.has(tag)) why.push(`<${tag}>`);
      if (why.length === 0) continue;
      const key = `${file}|${part}`;
      seen.add(key);
      if (KEPT.has(key)) continue;
      warnings.push(`  ${file}:${r.line}: ${part} restyles ${why.join(', ')} (${look.join(', ')})`);
    }
  }
}

const stale = [...KEPT.keys()].filter((k) => !seen.has(k));
if (warnings.length > 0) {
  console.log(`shared-look: ${warnings.length} page rule(s) change how a shared piece looks. For each, ask the person: make it an option of the piece, or keep it here with a reason.`);
  console.log(warnings.join('\n'));
} else {
  console.log('shared-look: no page rule changes how a shared piece looks');
}
for (const k of stale) console.log(`  kept rule gone, remove it from checks/shared-look.mjs: ${k}`);
