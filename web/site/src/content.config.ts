/** The site's Markdown content: text pages and cards, edited by MR. */
import { defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';
import { z } from 'astro/zod';

const pages = defineCollection({
  loader: glob({ pattern: '*.md', base: './src/content' }),
  schema: z.object({
    title: z.string(),
    /** Shown as "Last edited", as written. */
    updated: z.string(),
  }),
});

export const collections = { pages };
