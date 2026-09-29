/** The site's content: text pages and cards in MDX (Markdown with maths and components), edited by MR. */
import { defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';
import { z } from 'astro/zod';

const pages = defineCollection({
  loader: glob({ pattern: '*.mdx', base: './src/content' }),
  schema: z.object({
    title: z.string(),
    /** Shown as "Last edited", as written. */
    updated: z.string(),
  }),
});

const faq = defineCollection({
  loader: glob({ pattern: '*.mdx', base: './src/content/faq' }),
  schema: z.object({
    /** The tab's id in the address, e.g. /ratings/faq/#glicko-mp. */
    tab: z.string(),
    label: z.string(),
    order: z.number(),
  }),
});

export const collections = { pages, faq };
