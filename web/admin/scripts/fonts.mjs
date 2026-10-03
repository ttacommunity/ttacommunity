// Copies the site's committed fonts into admin's public folder for the build, so both use the same files.
import { cpSync, existsSync } from 'node:fs';

const from = new URL('../../site/public/fonts/', import.meta.url);
const to = new URL('../public/fonts/', import.meta.url);
if (!existsSync(new URL('fonts.css', from))) {
  console.error('fonts: the site has no public/fonts/fonts.css to copy');
  process.exit(1);
}
cpSync(from, to, { recursive: true });
console.log('fonts: copied the site fonts into admin');
