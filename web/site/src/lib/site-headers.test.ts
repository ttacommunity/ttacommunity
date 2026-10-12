import { describe, expect, it } from 'vitest';
import { siteHeaders } from './site-headers';

const template = `/*
  Content-Security-Policy: default-src 'self'; img-src 'self' data:; connect-src 'self' https://api.example; object-src 'none'
  X-Content-Type-Options: nosniff
`;

describe('siteHeaders', () => {
  it('points connect-src at the API origin it is built with', () => {
    const out = siteHeaders(template, 'https://api-dev.example', false);
    expect(out).toContain("connect-src 'self' https://api-dev.example; object-src 'none'");
    expect(out).not.toContain('https://api.example');
  });

  it('uses only the origin of an API URL with a path', () => {
    const out = siteHeaders(template, 'http://127.0.0.1:8080/v1/', false);
    expect(out).toContain("connect-src 'self' http://127.0.0.1:8080;");
  });

  it('leaves every other header and directive as it was', () => {
    const out = siteHeaders(template, 'https://api.example', false);
    expect(out).toBe(template);
  });

  it('adds noindex for every path when asked', () => {
    const out = siteHeaders(template, 'https://api.example', true);
    expect(out).toBe(template.replace('/*\n', '/*\n  X-Robots-Tag: noindex\n'));
  });

  it('puts noindex in the block for every path, not after a later block', () => {
    const out = siteHeaders(`${template}/flags/*\n  Cache-Control: max-age=60\n`, 'https://x.example', true);
    expect(out).toMatch(/^\/\*\n {2}X-Robots-Tag: noindex\n/);
    expect(out.match(/X-Robots-Tag/g)).toHaveLength(1);
  });

  it('refuses noindex without a block for every path', () => {
    expect(() => siteHeaders("/ratings/*\n  connect-src 'self'\n", 'https://x.example', true)).toThrow(/\/\*/);
  });

  it('refuses headers without a connect-src', () => {
    expect(() => siteHeaders("/*\n  Content-Security-Policy: default-src 'self'\n", 'https://x.example', false)).toThrow(
      /connect-src/,
    );
  });

  it('refuses an API URL that is not http or https', () => {
    expect(() => siteHeaders(template, 'javascript:alert(1)', false)).toThrow(/http/);
    expect(() => siteHeaders(template, 'not a url', false)).toThrow();
  });
});
