import { defineCollection } from 'astro:content';
import { docsLoader, i18nLoader } from '@astrojs/starlight/loaders';
import { docsSchema, i18nSchema } from '@astrojs/starlight/schema';

// Pages live under src/content/docs/docs/ so they are served at /docs/…,
// leaving /, /download and the legal pages to ordinary Astro pages and
// keeping the top level free for the hub's own paths.
export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
  // Declared so Starlight stops looking for it. Hindi and Telugu arrive with
  // the app's own translations, not before.
  i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema() }),
};
