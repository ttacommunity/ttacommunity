// @ts-check
import { defineConfig, envField } from 'astro/config';
import react from '@astrojs/react';

// Stylesheets are files, never inlined, so the CSP needs no 'unsafe-inline'.
export default defineConfig({
  integrations: [react()],
  build: { inlineStylesheets: 'never' },
  env: {
    schema: {
      // The public API the browser reads ratings from. The default is local development's.
      PUBLIC_API_URL: envField.string({ context: 'client', access: 'public', url: true, default: 'http://127.0.0.1:8080' }),
    },
  },
});
