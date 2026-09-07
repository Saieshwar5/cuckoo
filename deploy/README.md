# Deploying Cuckoo

The runbook for the one hub. Everything here is ours: the code is public for
transparency, but there is no supported way for anyone else to run it, so
these files are tuned for one machine and nothing else.

## What runs where

| Path | Served by |
| --- | --- |
| `/v1/*` | the hub — the client, agent and management APIs |
| `/p/*` | the hub — the page a scanned QR opens |
| `/a/*` | the hub — agent pictures, the only public bytes |
| `/healthz` | the hub — status, version, Postgres and Redis |
| everything else | the static site built from `web/` |

One machine runs five containers from `compose.yml`: the hub, Postgres, Redis,
Caddy, and the welcome agent. Caddy owns the certificate and the split above.

| On the server | What |
| --- | --- |
| `/srv/cuckoo/` | `compose.yml`, `Caddyfile`, `backup.sh`, and `.env` (the only file with secrets) |
| `/srv/www/` | the site, a folder of files |
| `/data/postgres/` | the database |
| `/data/media/` | every uploaded file, owned by uid 10001 |
| `/data/backups/` | nightly dumps, two weeks of them |

Nothing is compiled on the server. `deploy.sh` builds the images and the site
on your machine and ships the results.

## The first deploy, once

**The machine.** Ubuntu 24.04, two cores and four gigabytes, a block volume
mounted at `/data`, in an Indian region. SSH by key only, a firewall allowing
22, 80 and 443, automatic security updates. Then:

```bash
curl -fsSL https://get.docker.com | sh
apt-get install -y rsync
mkdir -p /srv/cuckoo /srv/www /data/postgres /data/media /data/backups
chown 10001:10001 /data/media          # the hub image runs as this user
```

**DNS.** An `A` record for the domain pointing at the machine. Caddy fetches
the certificate on first start, so the record must resolve before then.

**Secrets.** Copy `.env.example` to `/srv/cuckoo/.env` on the server and fill
in `SITE_DOMAIN` and `POSTGRES_PASSWORD` (`openssl rand -hex 24`). Leave the
image tags as they are; the deploy writes them.

**Deploy.** From a checkout on your laptop, with passwordless SSH to the host:

```bash
DEPLOY_HOST=root@203.0.113.4 SITE_DOMAIN=cuckoo.in deploy/deploy.sh
```

It builds both images tagged with the git version, builds the site for the
domain, ships the images over SSH with `docker save`, copies the files,
writes the tags into `.env`, runs `docker compose up -d`, and waits for
`https://<domain>/healthz` to answer with the version it just built.

**Check.** The health endpoint shows the version. The landing page loads. A
made-up pair link, `/p/nothing`, renders the hub's "not valid" card rather
than the site's 404. `/.well-known/` serves.

## The first sign-in and the welcome agent

Sign-in codes go to the hub's log until a mail sender exists, deliberately.

1. Point the app at the domain (`EXPO_PUBLIC_HUB_URL=https://<domain>`) and
   sign in. The six-digit code is in `docker compose logs -f hub`.
2. In the app, Settings → API keys → create one. Copy the `mgt_tok_…`.
3. On your laptop, create the welcome agent and connect it:

   ```bash
   export CUCKOO_HUB=https://<domain> CUCKOO_KEY=mgt_tok_...
   cuckoo agents create welcome "Cuckoo" \
     --description "Says hello, and how this works." \
     --starter "How do I add an agent?" --starter "How do I make my own?" --starter "What is this?"
   cuckoo agents connect <the agent id>        # prints bnd_sec_..., once
   ```

4. In `/srv/cuckoo/.env` set `WELCOME_HANDLE=welcome`, `WELCOME_SECRET=` to
   that secret, and `COMPOSE_PROFILES=welcome`. Then `docker compose up -d`.
   The hub restarts with the handle; the agent starts and connects.
5. Sign up with a second address. The welcome chat is there on arrival.

## Updating

Run `deploy.sh` again. Data is untouched; the hub applies any new migrations
as it starts. The previous images stay on the server, which is what makes the
next section work.

## Rolling back

```bash
DEPLOY_HOST=root@203.0.113.4 SITE_DOMAIN=cuckoo.in deploy/deploy.sh rollback <tag>
```

`<tag>` is a version the health endpoint showed before, or one from
`docker images cuckoo-hub` on the server. A migration that the older binary
does not know about is still in the database; the migrations so far are
additive, and the plan is to keep them that way.

## Logs

```bash
cd /srv/cuckoo
docker compose ps
docker compose logs -f hub            # JSON lines; the sign-in codes are here
docker compose logs --tail 200 caddy
```

Logs rotate at five files of twenty megabytes per service.

## Backups and restore

`backup.sh` dumps the database nightly, keeps two weeks locally, and copies
the dumps and the media folder to a bucket if `BACKUP_REMOTE` is set in
`.env`. Install rclone on the server and configure a remote for the bucket,
then:

```bash
echo '17 3 * * * /srv/cuckoo/backup.sh >> /var/log/cuckoo-backup.log 2>&1' | crontab -
/srv/cuckoo/backup.sh                 # once by hand, to see it work
```

**Restore, tested once before it matters.** On a scratch machine with the
same compose file:

```bash
docker compose up -d postgres
zcat cuckoo-<stamp>.sql.gz | docker compose exec -T postgres psql -U cuckoo -d cuckoo
rclone sync <remote>/media /data/media && chown -R 10001:10001 /data/media
docker compose up -d
```

## Trying the whole stack on a laptop

Point the paths and ports at scratch locations in a temporary `.env`, and use
a project name that is not the development stack's:

```bash
docker compose -p cuckoo-stacktest --env-file /tmp/stack.env -f deploy/compose.yml up -d
curl -k https://localhost:8443/healthz
docker compose -p cuckoo-stacktest --env-file /tmp/stack.env -f deploy/compose.yml down -v
```

with `SITE_DOMAIN=localhost`, `HTTP_PORT=8088`, `HTTPS_PORT=8443`,
`DATA_DIR` and `WWW_DIR` set to scratch directories. Caddy signs its own
certificate for `localhost`.

## App links

`web/public/.well-known/README.md` explains the two files that make a scanned
link open the app. They wait for the app's package name and signing
certificate, which arrive with the first store build.

## Not here yet

- **Mail.** Console is the only mode. An SMTP mailer and a sender with an
  Indian region arrive when the first person who is not you signs in.
- **Managed Postgres.** Worth buying at the same moment. It removes the
  `postgres` service from the compose file and changes one URL.
- **Push notifications** and **store builds** are roadmap items, not deploy
  items.
