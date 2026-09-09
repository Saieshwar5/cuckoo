# Deploying Cuckoo

The runbook for the one hub. Everything here is ours: the code is public for
transparency, but there is no supported way for anyone else to run it, so
these files are tuned for one deployment and nothing else.

The domain is **cuckoo.onl**. The region is **ap-south-1** (Mumbai).

## What runs where

| Path | Served by |
| --- | --- |
| `/v1/*` | the hub — the client, agent and management APIs |
| `/p/*` | the hub — the page a scanned QR opens |
| `/a/*` | the hub — agent pictures, the only public bytes |
| `/healthz` | the hub — status, version, Postgres and Redis |
| everything else | the static site built from `web/` |

One EC2 instance runs four containers from `compose.yml` — the hub, Redis,
Caddy and the welcome agent — with two AWS services beside them:

| | Where | Why |
| --- | --- | --- |
| the database | **RDS** PostgreSQL 16, private, TLS | snapshots and point-in-time recovery without a cron job |
| uploaded files | **S3**, private, versioned | the instance keeps no state worth losing |
| Redis | a **container** | stream buffers, rate-limit counters and the event bus are rebuilt from use — there is nothing for a managed cache to manage |

Bytes still travel through the hub: a bucket changes where a file lands, not
who serves it. Nobody is given a presigned URL, and the bucket is never public.

| On the instance | What |
| --- | --- |
| `/srv/cuckoo/` | `compose.yml`, `Caddyfile`, `backup.sh`, and `.env` (the only file with secrets) |
| `/srv/www/` | the site, a folder of files |
| `/data/backups/` | nightly dumps, two weeks of them |

That is the whole of the state on the machine, and it is two weeks of dumps
that also live in S3. There is no need for a separate EBS volume: a directory
on the root volume is enough. Nothing is compiled here either — `deploy.sh`
builds the images and the site on your laptop and ships the results.

## The first deploy, once

### The network, before anything else

Three security groups' worth of care, in this order:

1. **EC2 security group** — inbound `443` and `80` from anywhere, `22` from
   your address only. This is what Caddy and `deploy.sh` need.
2. **RDS security group** — inbound `5432` **from the EC2 security group's
   id**, not from a CIDR. The instance must be created with **Publicly
   accessible = No**.
3. **A VPC gateway endpoint for S3.** Free, two clicks, and it keeps every
   upload and every backup off the public internet.

Put the RDS instance in the **same availability zone** as the EC2 one:
cross-AZ traffic is charged and slower, and there is no availability being
bought by spreading two things that both have to be up.

### The instance

Ubuntu 24.04, `t3.medium` (2 vCPU / 4 GB), in ap-south-1.

**It must be x86_64, not Graviton.** `deploy.sh` builds images on your laptop
and ships them with `docker save`, so the image architecture is the laptop's.
An ARM instance would reject them. Building for ARM means adding
`--platform linux/arm64` and a qemu emulator to every build, which is slow;
the saving is not worth it.

**Launch it with a metadata hop limit of 2.** The hub reaches S3 with the
instance's IAM role, and it reads that role from the instance metadata
service — but it runs in a container, and Docker's bridge network adds one
hop. With the default limit of 1 the credentials are simply unreachable and
every upload fails at run time with an access error that looks like a policy
mistake. In the launch wizard it is *Advanced details → Metadata response hop
limit → 2*, or:

```bash
aws ec2 modify-instance-metadata-options --instance-id i-... \
  --http-tokens required --http-put-response-hop-limit 2
```

**An IAM role on the instance**, with one policy over the two buckets and
nothing else:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": ["arn:aws:s3:::cuckoo-media/*", "arn:aws:s3:::cuckoo-backups/*"] },
    { "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": ["arn:aws:s3:::cuckoo-media", "arn:aws:s3:::cuckoo-backups"] }
  ]
}
```

`ListBucket` is not decoration: without it S3 answers a missing object with an
access denial rather than "no such key", and a file the retention sweep has
already deleted would read as a server fault instead of a gone file.

Then, on the machine:

```bash
curl -fsSL https://get.docker.com | sh
sudo apt-get install -y rsync
sudo usermod -aG docker ubuntu          # deploy.sh runs docker as this user
sudo mkdir -p /srv/cuckoo /srv/www /data/backups
sudo chown -R ubuntu:ubuntu /srv/cuckoo /srv/www /data
```

`deploy.sh` connects as `ubuntu` and uses neither `sudo` nor a root login, so
those two lines are what make it work: the directories are the deploy user's,
and the deploy user may talk to Docker. Log out and back in for the group to
take effect.

SSH by key only, and unattended security updates on.

### The database

RDS PostgreSQL **16** — the same major version as the container the tests run
against — `db.t4g.micro` to start, 20 GB gp3, **Single-AZ**, automated backups
on with seven days of retention. The master user is `cuckoo` and the database
is `cuckoo`; nothing needs a superuser, and no migration creates an extension.

`DB_SSLMODE=require` encrypts the connection without checking who answered,
which is what a private subnet already settles. To check as well, download the
ap-south-1 RDS root certificate, mount it into the hub container, and set
`DB_SSLMODE=verify-full` with `&sslrootcert=` naming it.

**Prove the security group before deploying.** A failed deploy is confusing;
a failed `psql` is obvious:

```bash
docker run --rm -it -e PGPASSWORD=... postgres:16-alpine \
  psql -h <rds endpoint> -U cuckoo cuckoo -c 'select 1'
```

### The buckets

`cuckoo-media` and `cuckoo-backups`, both with **Block Public Access on** and
**versioning on**. A lifecycle rule on `cuckoo-backups` expiring objects after
30 days; nothing on `cuckoo-media`, because the hub's own retention sweep is
what deletes files there.

### DNS

An `A` record for `cuckoo.onl` pointing at the instance's elastic IP. Caddy
fetches the certificate on first start, so the record must resolve before
then.

### Secrets

Copy `.env.example` to `/srv/cuckoo/.env` and fill in `DB_HOST`, `DB_PASSWORD`
and `SIGNING_KEY` (`openssl rand -hex 32`), the key the hub signs every message
with. Leave the image tags as they are; the deploy writes them.

**Keep the signing key somewhere off the machine.** There is one key and no
rotation: change it and every stored signature stops verifying, and it is not
in the database backup.

Note what is *not* in there: no AWS access key. Storage works through the
instance's role.

### Deploy

From a checkout on your laptop, with passwordless SSH to the host:

```bash
DEPLOY_HOST=ubuntu@203.0.113.4 SITE_DOMAIN=cuckoo.onl deploy/deploy.sh
```

It builds both images tagged with the git version, builds the site for the
domain, ships the images over SSH with `docker save`, copies the files,
writes the tags into `.env`, runs `docker compose up -d`, and waits for
`https://cuckoo.onl/healthz` to answer with the version it just built.

### Check

The health endpoint shows the version. The landing page loads. A made-up pair
link, `/p/nothing`, renders the hub's "not valid" card rather than the site's
404. `/.well-known/` serves. And in `docker compose logs hub`, the storage
line says the bucket:

```
media storage ready  backend=s3  bucket=cuckoo-media
```

If it says `backend=disk`, `.env` did not reach the container. If start-up
fails on credentials, it is the metadata hop limit.

## The first sign-in and the welcome agent

Sign-in codes go to the hub's log until a mail sender is configured,
deliberately.

1. Point the app at the domain (`EXPO_PUBLIC_HUB_URL=https://cuckoo.onl`) and
   sign in. The six-digit code is in `docker compose logs -f hub`.
2. In the app, Settings → API keys → create one. Copy the `mgt_tok_…`.
3. On your laptop, create the welcome agent and connect it:

   ```bash
   export CUCKOO_HUB=https://cuckoo.onl CUCKOO_KEY=mgt_tok_...
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
as it starts. The previous images stay on the instance, which is what makes
the next section work.

## Rolling back

```bash
DEPLOY_HOST=ubuntu@203.0.113.4 SITE_DOMAIN=cuckoo.onl deploy/deploy.sh rollback <tag>
```

`<tag>` is a version the health endpoint showed before, or one from
`docker images cuckoo-hub` on the instance. A migration that the older binary
does not know about is still in the database; the migrations so far are
additive, and the plan is to keep them that way — it is the only reason a
rollback is one command.

## Logs

```bash
cd /srv/cuckoo
docker compose ps
docker compose logs -f hub            # JSON lines; the sign-in codes are here
docker compose logs --tail 200 caddy
```

Logs rotate at five files of twenty megabytes per service.

## Retention

The hub keeps a message and its files for 90 days, then deletes them, oldest
first — from the bucket as well as the database. A person may hold 100 MB of
uploaded files on live messages and an agent 1 GB; over that, the oldest files
go and the message text stays. Delivery records are deleted a week after they
reach a final state, and a sweep runs inside the hub every six hours. The
numbers are `CUCKOO_RETENTION_DAYS` (0 keeps everything),
`CUCKOO_USER_MEDIA_BUDGET_MB` and `CUCKOO_AGENT_MEDIA_BUDGET_MB` in `.env`;
changing one is an edit and `docker compose up -d`.
`CUCKOO_RETENTION_DRY_RUN=true` makes the sweep log what it would delete and
delete nothing, which is the way to check a new number before it runs for real.

Bucket versioning means a swept file is still recoverable for as long as the
bucket's own lifecycle rule keeps the old version. That is a deliberate gap
between what the hub promises and what AWS holds; close it with a lifecycle
rule that expires non-current versions if the promise ever has to be exact.

## Backups and restore

RDS takes its own snapshots. `backup.sh` takes a logical dump as well, because
a snapshot restores an instance while a dump can be read, searched, restored
into a scratch database on a laptop, and loaded into a Postgres that is not
RDS at all. It runs `pg_dump` and the AWS CLI in throwaway containers, so the
machine needs neither installed.

```bash
echo '17 3 * * * /srv/cuckoo/backup.sh >> /var/log/cuckoo-backup.log 2>&1' | crontab -
/srv/cuckoo/backup.sh                 # once by hand, to see it work
```

Uploaded files are not copied: they are in S3 with versioning on already.

**Restore, tested once before it matters.** Against a scratch RDS instance, or
a Postgres container on your laptop:

```bash
createdb -h <scratch host> -U cuckoo cuckoo_restore
zcat cuckoo-<stamp>.sql.gz | psql -h <scratch host> -U cuckoo -d cuckoo_restore
```

Then point a hub at it with `CUCKOO_BLOBS=s3` and the same bucket, and open a
chat with a picture in it. A restore that has not been read is a guess.

## Trying the whole stack on a laptop

`compose.local.yml` puts the database and the files back on the machine, which
is what a laptop has instead of AWS. Use a project name that is not the
development stack's:

```bash
docker compose -p cuckoo-stacktest --env-file /tmp/stack.env \
  -f deploy/compose.yml -f deploy/compose.local.yml up -d
curl -k https://localhost:8443/healthz
docker compose -p cuckoo-stacktest --env-file /tmp/stack.env \
  -f deploy/compose.yml -f deploy/compose.local.yml down -v
```

with `SITE_DOMAIN=localhost`, `HTTP_PORT=8088`, `HTTPS_PORT=8443`, any
`DB_PASSWORD`, a throwaway `SIGNING_KEY` (`openssl rand -hex 32`), `DATA_DIR`
and `WWW_DIR` set to scratch directories, and the three settings that decide
where state lives:

```
DB_HOST=postgres
DB_SSLMODE=disable
CUCKOO_BLOBS=disk
```

Those go in the env file rather than the overlay because compose reads `${...}`
before it merges files. Caddy signs its own certificate for `localhost`.

## Moving files between disk and a bucket

Both stores use the same keys, so this is a copy and a setting, never a
migration — nothing in the database names a backend:

```bash
aws s3 sync /data/media s3://cuckoo-media/          # to the bucket
aws s3 sync s3://cuckoo-media/ /data/media          # back again
```

Then change `CUCKOO_BLOBS`, add or drop the overlay, and `docker compose up -d`.

## App links

`web/public/.well-known/README.md` explains the two files that make a scanned
link open the app. They wait for the app's package name and signing
certificate, which arrive with the first store build.

## Mail

Sign-in codes go to the hub's log until a provider is configured, which is
enough while you are the only person signing in. For anybody else:

1. **An account with a sending provider.** Amazon's mail service in the
   Mumbai region is the closest and cheapest, and it is already in the
   account; Postmark, Resend and Brevo set up faster and cost more. Either way
   it starts restricted to addresses you have verified, and lifting that is a
   short form and about a day. Start it early: it is the one step that waits
   on somebody else.
2. **Three DNS records on cuckoo.onl**: SPF, DKIM and DMARC, as the provider
   gives them. Without these the codes land in spam folders, and a sign-in
   code in a spam folder is a person who gives up.
3. **Six settings in `.env`**: `CUCKOO_MAIL=smtp` plus the host, port,
   username, password and from address. Port 587 upgrades with STARTTLS and
   465 is TLS from the start; the hub refuses anything else rather than send
   a code in the clear.
4. **Send to a real inbox at each of Gmail, Outlook, Yahoo and Rediffmail**
   before calling it done. Indian inboxes do not all behave the same.

The hub logs where it is sending from at start-up, so a misconfigured
provider is visible before anybody tries to sign in.

## Not here yet
- **Push notifications** and **store builds** are roadmap items, not deploy
  items.
- **A second instance.** Nothing on the machine is state any more, so this is
  a load balancer and a second `deploy.sh` target when it is ever needed.
