# Deploying

Two things run in front of the world: the hub, and the website. One domain
covers both, because a pair link is printed on posters and
`cuckoo.in/p/abc123` is a better thing to print than a second hostname.

## The split

| Path | Served by |
| --- | --- |
| `/v1/*` | the hub — the client, agent and management APIs |
| `/p/*` | the hub — the page a scanned QR opens |
| `/a/*` | the hub — agent pictures, the only public bytes |
| `/healthz` | the hub |
| everything else | the static site built from `web/` |

`deploy/Caddyfile` is that rule, and it also fetches the TLS certificate.

## Before the first deploy

1. **Choose the domain.** It is still undecided, and it is baked into pair
   links, the site's canonical URLs and the app-link files. Set it once in
   `SITE_DOMAIN`, in `web/src/config.ts` (or the `SITE_URL` environment
   variable at build time), and in the hub's public URL setting.
2. **Point DNS at the machine.** An `A` record is enough.
3. **Set a mail sender** on the hub, or nobody can sign in.

## Building the site

```bash
cd web
npm ci
SITE_URL=https://cuckoo.in npm run build     # writes web/dist
```

Copy `web/dist` to `/srv/www` on the server. It is plain files; there is no
runtime.

## App links

`web/public/.well-known/` holds two example files and instructions. They cannot
be filled in until the app has a package name, a bundle id and a signing
certificate, which happens at the first store build. Until then a scanned link
opens the pair page in a browser, which offers to open the app.

## What to watch

- `/healthz` reports the hub, Postgres and Redis.
- Postgres is the system of record. Back it up.
- The blob directory holds every uploaded file. Back it up too.
