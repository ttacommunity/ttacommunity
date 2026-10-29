// @ts-check
import { defineConfig, envField } from 'astro/config';
import react from '@astrojs/react';
import mdx from '@astrojs/mdx';
import { unified } from '@astrojs/markdown-remark';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';

// Stylesheets are files, never inlined, so the CSP needs no 'unsafe-inline'.
export default defineConfig({
  integrations: [react(), mdx()],
  // Maths in Markdown is rendered at build, as MathML: no script, no inline styles for the CSP to allow.
  // Straight quotes, as upstream's Markdown renderer leaves them. No code highlighting: it colours with inline styles.
  markdown: {
    syntaxHighlight: false,
    processor: unified({
      smartypants: false,
      remarkPlugins: [remarkMath],
      rehypePlugins: [[rehypeKatex, { output: 'mathml' }]],
    }),
  },
  build: { inlineStylesheets: 'never' },
  // Our scripts are files too, so editing one never changes the CSP's hashes.
  vite: { build: { assetsInlineLimit: (file) => (file.endsWith('.js') ? false : undefined) } },
  env: {
    schema: {
      // The public API the browser reads ratings from. The default is local development's.
      PUBLIC_API_URL: envField.string({ context: 'client', access: 'public', url: true, default: 'http://127.0.0.1:8080' }),
      // The admin site the header and home page link to. The default is local development's.
      PUBLIC_ADMIN_URL: envField.string({ context: 'client', access: 'public', url: true, default: 'http://127.0.0.1:4322' }),
      // The site's own address, for the absolute links link previews need (a challenge's picture). The default is local development's.
      PUBLIC_SITE_URL: envField.string({ context: 'server', access: 'public', url: true, default: 'http://127.0.0.1:4321' }),
      // The email form's Turnstile widget. The default is Cloudflare's test key, which passes every check; the api
      // refuses its tokens unless it runs for local development.
      PUBLIC_TURNSTILE_SITEKEY: envField.string({ context: 'client', access: 'public', default: '1x00000000000000000000AA' }),
    },
  },
});
