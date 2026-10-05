// The site's fonts, committed under public/fonts/: which faces it uses, how
// Google Fonts describes them, and the stylesheet that serves our copies.
// scripts/fonts.mjs checks Google for newer files and updates our copies.

/** A family the site uses: Google's css2 axis query and the subsets kept. */
export interface FontFamily {
  family: string;
  axes: string;
  subsets: string[];
}

/** FONTS are upstream's fonts, at the weights and styles the site uses. */
export const FONTS: FontFamily[] = [
  { family: 'Cinzel', axes: ':wght@500;700;900', subsets: ['latin'] },
  { family: 'Marcellus', axes: '', subsets: ['latin'] },
  { family: 'Source Serif 4', axes: ':ital,wght@0,400;0,600;0,700;1,400;1,600;1,700', subsets: ['latin', 'latin-ext'] },
  { family: 'Inter', axes: ':wght@400;500;600;700', subsets: ['latin', 'latin-ext'] },
];

/** A face as Google serves it: one family, style, weight and subset, in one file. */
export interface Face {
  family: string;
  style: string;
  weight: string;
  subset: string;
  url: string;
  unicodeRange: string;
}

/** googleCssUrl returns the Google Fonts css2 address for every family in fonts. */
export function googleCssUrl(fonts: FontFamily[]): string {
  const url = new URL('https://fonts.googleapis.com/css2');
  for (const f of fonts) url.searchParams.append('family', f.family + f.axes);
  url.searchParams.set('display', 'swap');
  return url.toString();
}

/** parseGoogleCss returns each face in Google's css2 response; it throws if there are none. */
export function parseGoogleCss(css: string): Face[] {
  const faces: Face[] = [];
  for (const [, subset, body] of css.matchAll(/\/\*\s*([\w-]+)\s*\*\/\s*@font-face\s*\{([^}]*)\}/g)) {
    const prop = (name: string) => body.match(new RegExp(`${name}:\\s*([^;]+);`))?.[1].trim() ?? '';
    faces.push({
      family: prop('font-family').replace(/^'|'$/g, ''),
      style: prop('font-style'),
      weight: prop('font-weight'),
      subset,
      url: prop('src').match(/url\(([^)]+)\)/)?.[1] ?? '',
      unicodeRange: prop('unicode-range'),
    });
  }
  if (faces.length === 0) throw new Error('no @font-face in the response: not Google Fonts CSS');
  return faces;
}

/** pick keeps the faces of each family's configured subsets. */
export function pick(faces: Face[], fonts: FontFamily[]): Face[] {
  return faces.filter((f) => fonts.some((x) => x.family === f.family && x.subsets.includes(f.subset)));
}

/** fontFile names our copy of a face: its family, then Google's file name. */
export function fontFile(face: Face): string {
  const slug = face.family.toLowerCase().replace(/[^a-z0-9]+/g, '-');
  return `${slug}-${face.url.split('/').pop()}`;
}

const key = (f: Face) => `${f.family} ${f.weight} ${f.style} ${f.subset}`;

/** changes lists, in order, each face that Google now serves from a new file, adds, or no longer serves. */
export function changes(ours: Face[], google: Face[]): string[] {
  const had = new Map(ours.map((f) => [key(f), f.url]));
  const has = new Map(google.map((f) => [key(f), f.url]));
  const out: string[] = [];
  for (const [k, url] of has) {
    if (!had.has(k)) out.push(`${k}: new`);
    else if (had.get(k) !== url) out.push(`${k}: new file`);
  }
  for (const k of had.keys()) if (!has.has(k)) out.push(`${k}: gone`);
  return out.sort();
}

/** renderCss returns the stylesheet serving each face from /fonts/. */
export function renderCss(faces: Face[]): string {
  const blocks = faces.map(
    (f) =>
      `/* ${f.subset} */\n@font-face {\n  font-family: '${f.family}';\n  font-style: ${f.style};\n  font-weight: ${f.weight};\n` +
      `  font-display: swap;\n  src: url('/fonts/${fontFile(f)}') format('woff2');\n  unicode-range: ${f.unicodeRange};\n}`,
  );
  return `/* Written by scripts/fonts.mjs from Google Fonts; do not edit. */\n${blocks.join('\n')}\n`;
}
