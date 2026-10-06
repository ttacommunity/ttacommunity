// @ts-check
import { defineConfig } from 'astro/config';
import react from '@astrojs/react';

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
        },
      },
    },
  },
  integrations: [react()],
  build: { inlineStylesheets: 'never' },
});

