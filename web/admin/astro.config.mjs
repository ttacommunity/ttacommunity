// @ts-check
import { defineConfig } from 'astro/config';
import react from '@astrojs/react';

// Stylesheets are files, never inlined, so the CSP needs no 'unsafe-inline'.
export default defineConfig({
  integrations: [react()],
  build: { inlineStylesheets: 'never' },
});
