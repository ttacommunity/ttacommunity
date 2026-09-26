// ESLint for TypeScript and React files. `.astro` files are checked by `astro check`.
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['**/dist/**', '**/.astro/**', '**/node_modules/**'] },
  ...tseslint.configs.strict,
);
