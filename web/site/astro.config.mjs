// @ts-check
import { defineConfig, envField, fontProviders } from 'astro/config';
import react from '@astrojs/react';
import mdx from '@astrojs/mdx';
import { unified } from '@astrojs/markdown-remark';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';

const google = fontProviders.google();

// Stylesheets are files, never inlined, so the CSP needs no 'unsafe-inline'.
export default defineConfig({
  integrations: [react(), mdx()],
  // Maths in Markdown is rendered at build, as MathML: no script, no inline styles for the CSP to allow.
  // Straight quotes, as upstream's Markdown renderer leaves them.
  markdown: {
    processor: unified({
      smartypants: false,
      remarkPlugins: [remarkMath],
      rehypePlugins: [[rehypeKatex, { output: 'mathml' }]],
    }),
  },
  build: { inlineStylesheets: 'never' },
  // Our scripts are files too, so editing one never changes the CSP's hashes.
  vite: { build: { assetsInlineLimit: (file) => (file.endsWith('.js') ? false : undefined) } },
  // Upstream's Google fonts, downloaded at build and served from this site.
  fonts: [
    { provider: google, name: 'Cinzel', cssVariable: '--font-cinzel', weights: [500, 700, 900], fallbacks: ['serif'] },
    { provider: google, name: 'Marcellus', cssVariable: '--font-marcellus', fallbacks: ['serif'] },
    {
      provider: google,
      name: 'Source Serif 4',
      cssVariable: '--font-source-serif',
      weights: [400, 600, 700],
      styles: ['normal', 'italic'],
      subsets: ['latin', 'latin-ext'],
      fallbacks: ['Georgia', 'serif'],
    },
    {
      provider: google,
      name: 'Inter',
      cssVariable: '--font-inter',
      weights: [400, 500, 600, 700],
      subsets: ['latin', 'latin-ext'],
      fallbacks: ['system-ui', 'sans-serif'],
    },
  ],
  env: {
    schema: {
      // The public API the browser reads ratings from. The default is local development's.
      PUBLIC_API_URL: envField.string({ context: 'client', access: 'public', url: true, default: 'http://127.0.0.1:8080' }),
    },
  },
});
