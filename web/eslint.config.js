// ESLint for TypeScript, React and Astro files, with the strict accessibility rules.
import tseslint from 'typescript-eslint';
import astro from 'eslint-plugin-astro';
import jsxA11y from 'eslint-plugin-jsx-a11y';

export default tseslint.config(
  { ignores: ['**/dist/**', '**/.astro/**', '**/node_modules/**'] },
  ...tseslint.configs.strict,
  ...astro.configs.recommended,
  ...astro.configs['jsx-a11y-strict'],
  { files: ['**/*.tsx'], ...jsxA11y.configs.strict },
);
