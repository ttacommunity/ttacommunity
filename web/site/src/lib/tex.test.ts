import { describe, expect, it } from 'vitest';
import katex from 'katex';
import { TEX_OPTIONS } from './tex';

describe('TEX_OPTIONS', () => {
  it('render MathML with the TeX kept, and no inline styles for the CSP to refuse', () => {
    const html = katex.renderToString('r_s > 0.95', TEX_OPTIONS);
    expect(html).toContain('<math');
    expect(html).toContain('<annotation encoding="application/x-tex">r_s &gt; 0.95</annotation>');
    expect(html).not.toContain('style=');
  });

  it('throw on TeX KaTeX cannot read, rather than print an error in an inline style', () => {
    expect(() => katex.renderToString('\\frac{1}{', TEX_OPTIONS)).toThrow();
  });
});
