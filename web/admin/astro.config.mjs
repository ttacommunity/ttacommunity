// @ts-check
import { defineConfig } from 'astro/config';
import react from '@astrojs/react';
// With its .ts, Node loads it; without, Astro loads this file through Vite, which breaks admin's React in dev.
import { passBreaksOn } from './src/lib/dev-proxy.ts';

/** @typedef {import('./src/lib/dev-proxy.ts').Proxy} Proxy */

// Stylesheets are files, never inlined, so the CSP needs no 'unsafe-inline'.
export default defineConfig({
  server: { port: 4322, host: '127.0.0.1' },
  vite: {
    server: {
      proxy: {
        '/api': {
          target: 'http://127.0.0.1:8080/v1/admin',
          rewrite: (path) => path.replace(/^\/api/, ''),
          changeOrigin: true,
          headers: {
            Origin: 'http://127.0.0.1:4322',
          },
          // Vite's proxy types need @types/node, which web leaves out, hence the cast.
          configure: (proxy) => passBreaksOn(/** @type {Proxy} */ (/** @type {unknown} */ (proxy))),
        },
      },
    },
  },
  integrations: [react()],
  build: { inlineStylesheets: 'never' },
});

