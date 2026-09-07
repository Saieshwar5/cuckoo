// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

import { siteUrl, site } from './src/config';
import { codeDark, codeLight } from './src/styles/code-theme';

// The docs are Starlight; the landing, download and legal pages are plain
// Astro pages in src/pages, which take precedence over anything Starlight
// injects. Nothing here needs JavaScript in the browser except the copy
// buttons, so most of the site ships as HTML and CSS alone.
export default defineConfig({
  site: siteUrl,
  integrations: [
    starlight({
      title: site.name,
      description: site.description,
      logo: {
        light: './src/assets/logo-light.png',
        dark: './src/assets/logo-dark.png',
        replacesTitle: false,
        alt: 'Cuckoo',
      },
      favicon: '/favicon.png',
      customCss: ['./src/styles/cuckoo.css'],
      // Highlighting in greys, matching the rest of the site. See
      // src/styles/code-theme.ts for why.
      expressiveCode: {
        themes: [codeDark, codeLight],
        styleOverrides: {
          borderRadius: '14px',
          borderColor: 'var(--sl-color-hairline)',
          codeFontSize: '0.875rem',
          codeLineHeight: '1.65',
          frames: {
            shadowColor: 'transparent',
            editorTabBarBorderBottomColor: 'var(--sl-color-hairline)',
          },
        },
      },
      social: [{ icon: 'github', label: 'GitHub', href: site.repo }],
      editLink: { baseUrl: `${site.repo}/edit/main/web/` },
      lastUpdated: true,
      // Starlight would otherwise own /404; ours is an ordinary page so the
      // landing and the docs fail the same way.
      disable404Route: true,
      credits: false,
      titleDelimiter: '·',
      sidebar: [
        {
          label: 'Start here',
          items: [
            { slug: 'docs', label: 'Overview' },
            { slug: 'docs/quickstart' },
            { slug: 'docs/concepts' },
          ],
        },
        {
          label: 'Build an agent',
          items: [
            { slug: 'docs/your-own-agent' },
            { slug: 'docs/for-companies' },
            { slug: 'docs/identity' },
            { slug: 'docs/examples' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { slug: 'docs/protocol' },
            { slug: 'docs/sdk-python' },
            { slug: 'docs/cli' },
            { slug: 'docs/limits' },
          ],
        },
        // One page, so it is a link rather than a group of one.
        { slug: 'docs/open-source' },
      ],
    }),
  ],
});
