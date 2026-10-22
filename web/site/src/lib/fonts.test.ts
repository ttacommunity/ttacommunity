import { describe, expect, it } from 'vitest';
import { FONTS, changes, fontFile, googleCssUrl, parseGoogleCss, pick, renderCss, type Face } from './fonts';

const css = `/* latin-ext */
@font-face {
  font-family: 'Source Serif 4';
  font-style: italic;
  font-weight: 400;
  font-display: swap;
  src: url(https://fonts.gstatic.com/s/sourceserif4/v14/aaa.woff2) format('woff2');
  unicode-range: U+0100-02BA, U+1E00-1E9F;
}
/* latin */
@font-face {
  font-family: 'Source Serif 4';
  font-style: italic;
  font-weight: 400;
  font-display: swap;
  src: url(https://fonts.gstatic.com/s/sourceserif4/v14/bbb.woff2) format('woff2');
  unicode-range: U+0000-00FF, U+2000-206F;
}
/* cyrillic */
@font-face {
  font-family: 'Cinzel';
  font-style: normal;
  font-weight: 700;
  font-display: swap;
  src: url(https://fonts.gstatic.com/s/cinzel/v26/ccc.woff2) format('woff2');
  unicode-range: U+0400-045F;
}
/* latin */
@font-face {
  font-family: 'Cinzel';
  font-style: normal;
  font-weight: 700;
  font-display: swap;
  src: url(https://fonts.gstatic.com/s/cinzel/v26/ddd.woff2) format('woff2');
  unicode-range: U+0000-00FF;
}
`;

const face = (family: string, style: string, weight: string, subset: string, url: string): Face => ({
  family, style, weight, subset, url, unicodeRange: 'U+0000-00FF',
});

describe('googleCssUrl', () => {
  it('asks for every family, weight and style the site uses', () => {
    const url = new URL(googleCssUrl(FONTS));
    expect(url.origin + url.pathname).toBe('https://fonts.googleapis.com/css2');
    expect(url.searchParams.getAll('family')).toEqual([
      'Cinzel:wght@500;700;900',
      'Marcellus',
      'Source Serif 4:ital,wght@0,400;0,600;0,700;1,400;1,600;1,700',
      'Inter:wght@400;500;600;700',
      'Google Sans:wght@500',
    ]);
    expect(url.searchParams.get('display')).toBe('swap');
  });
});

describe('parseGoogleCss', () => {
  it('reads each face with its subset, file and range', () => {
    expect(parseGoogleCss(css)[0]).toEqual({
      family: 'Source Serif 4', style: 'italic', weight: '400', subset: 'latin-ext',
      url: 'https://fonts.gstatic.com/s/sourceserif4/v14/aaa.woff2', unicodeRange: 'U+0100-02BA, U+1E00-1E9F',
    });
    expect(parseGoogleCss(css)).toHaveLength(4);
  });
  it('refuses CSS with no faces, so an error page is never taken as fonts', () => {
    expect(() => parseGoogleCss('<html>Too many requests</html>')).toThrow(/no @font-face/);
  });
});

describe('pick', () => {
  it('keeps only the subsets each family is configured for', () => {
    const got = pick(parseGoogleCss(css), FONTS).map((f) => `${f.family} ${f.subset}`);
    expect(got).toEqual(['Source Serif 4 latin-ext', 'Source Serif 4 latin', 'Cinzel latin']);
  });
});

describe('fontFile', () => {
  it('names a file by its family and Google file name', () => {
    expect(fontFile(face('Source Serif 4', 'italic', '400', 'latin', 'https://fonts.gstatic.com/s/x/v14/bbb.woff2'))).toBe(
      'source-serif-4-bbb.woff2',
    );
  });
});

describe('changes', () => {
  const old = [face('Inter', 'normal', '400', 'latin', 'https://g/v1/a.woff2'), face('Inter', 'normal', '700', 'latin', 'https://g/v1/b.woff2')];
  it('is empty when Google serves the same files', () => {
    expect(changes(old, [...old].reverse())).toEqual([]);
  });
  it('names each face that is new, gone or served from a new file', () => {
    const now = [face('Inter', 'normal', '400', 'latin', 'https://g/v2/a.woff2'), face('Inter', 'normal', '500', 'latin', 'https://g/v1/c.woff2')];
    expect(changes(old, now)).toEqual(['Inter 400 normal latin: new file', 'Inter 500 normal latin: new', 'Inter 700 normal latin: gone']);
  });
});

describe('renderCss', () => {
  it('writes an @font-face per face, served from /fonts/', () => {
    const got = renderCss([{ ...face('Cinzel', 'normal', '700', 'latin', 'https://g/v26/ddd.woff2'), unicodeRange: 'U+0000-00FF' }]);
    expect(got).toContain(
      "@font-face {\n  font-family: 'Cinzel';\n  font-style: normal;\n  font-weight: 700;\n  font-display: swap;\n" +
        "  src: url('/fonts/cinzel-ddd.woff2') format('woff2');\n  unicode-range: U+0000-00FF;\n}",
    );
    expect(got).not.toContain('gstatic');
  });
});
