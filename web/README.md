# The website

The public face of Cuckoo: the landing page, the documentation, the download
page and the legal pages. Static files, no runtime, no database.

```bash
npm install
npm run dev        # http://localhost:4321
npm run build      # writes dist/
npm run preview    # serve dist/ as it will be served
```

## What is where

| Path | What |
| --- | --- |
| `src/pages/` | The landing page, download, 404 and the legal pages |
| `src/content/docs/docs/` | Every documentation page, as Markdown |
| `src/components/` | The header, the footer, and the phone drawn in CSS |
| `src/styles/cuckoo.css` | The monochrome palette, shared by the docs and the rest |
| `src/config.ts` | The domain, the download links, the contact address |
| `public/.well-known/` | App link files, and how to fill them in |

The documentation lives under `src/content/docs/docs/` so it is served at
`/docs/…`, leaving the top level for the hub's own paths and the landing page.

## Changing things

**The domain.** `src/config.ts`, or `SITE_URL` at build time. It is still
undecided, and it appears in canonical URLs, the sitemap and every code sample.

**The app links.** `downloads` in `src/config.ts`. Each platform has an
`available` flag, so the download page falls back to an honest "coming" line
until a real link exists. Flip the flag and add the URL when the store listing
is live.

**The sidebar.** `astro.config.ts`. A new page needs a file and a line there.

**The palette.** `src/styles/cuckoo.css` maps the app's own tokens
(`apps/mobile/src/theme/tokens.ts`) onto Starlight's scale. If the app's
palette changes, change it here too so a screenshot and the page around it
agree.

## Rules for the documentation

- **Only document what is built.** The internal design notes describe things
  the server does not do. Every page here was checked against the running code.
  If you add a page, check it the same way.
- **Every page starts with something that runs.** A reader who copies the first
  code block should get a working result.
- **Say what is missing.** A paragraph naming what does not exist yet saves
  somebody an afternoon and costs three lines.
- **No colour.** Links are underlined, never coloured. The one thing that gets
  a colour is an error.

## The phone on the landing page

`src/components/Phone.astro` draws a chat in CSS rather than showing a
screenshot: it is sharp at any size, follows the theme, and cannot fall out of
date. When there are real screenshots from a device worth showing, they belong
beside it, not instead of it.

## Deploying

See `deploy/README.md` at the root of the repository. `deploy/deploy.sh`
builds the site with the real domain and copies `dist/` to the server, where
Caddy routes `/v1`, `/p`, `/a` and `/healthz` to the hub and everything else to
the files.
