# runtime — the agents Cuckoo runs

One service that answers for every agent Cuckoo runs: the ready-made ones
anyone adds in a tap, and the custom ones a person configures in the app.

It is **system 2** (`proj-docs/09-hosted-runtime.md`): a company on the hub that
happens to be run by Cuckoo. It reaches the hub only through the public
protocol — `/v1/agent` and `/v1/mgmt`, the same APIs any company uses — and has
no private door into it. If it ever needs one, the protocol is what is
incomplete.

## The shape of it

**One process, every agent.** An agent is a row, not a running thing. Nothing
is started or held open per agent, so serving ten thousand costs what serving
ten does plus the messages. A second copy of the process is how it scales: the
queue's `SKIP LOCKED` means two workers share the work rather than duplicating
it.

**Webhooks, not sockets.** The hub posts each event to `/hooks/{agentId}`. The
agent is named by the URL, so the secret to verify with is chosen before a byte
of the body is believed — and a body whose own `agent_id` disagrees is refused.

**Answer first, think after.** The hub gives a backend ten seconds and retries
anything slower; a model takes longer than that. So the door verifies, writes a
job down, and answers. The worker calls the model afterwards. The unique index
on `event_id` is the whole de-duplication story: a redelivery inserts nothing.

**Nothing is forgotten.** Every turn — what was said, every tool call, every
result — is kept here forever. What the model *sees* is a window assembled per
run. The two are different things, and conflating them is how an agent ends up
with the memory of a goldfish. (Compaction and recall are the next step; the
rows are already shaped for them.)

**pi is behind one file.** `src/harness/pi.ts` is the only place that knows pi
exists. Everything above speaks `HarnessEvent`. pi is young and moves fast, so
that seam is a day's work rather than a week's.

```
hub ──POST /hooks/agt_…──▶ verify ─▶ jobs row ─▶ 200          (milliseconds)
                                        │
                              worker ───┘
                                 ├─ load persona + window + tools
                                 ├─ run the model (pi → Claude)
                                 ├─ stream text back to the hub as it arrives
                                 └─ write the turn down, forget the rest
```

## Running it

```bash
npm install
npm test          # 35 tests; the database ones need postgres on :5433 (make up)
npm run typecheck
```

```bash
export RUNTIME_DATABASE_URL=postgres://cuckoo:cuckoo@localhost:5433/cuckoo_runtime
export RUNTIME_SECRET_KEY=$(openssl rand -hex 32)
export RUNTIME_HUB_URL=http://localhost:8080
export ANTHROPIC_API_KEY=sk-ant-…
npm start
```

| Setting | What |
| --- | --- |
| `RUNTIME_DATABASE_URL` | Its own Postgres. Never the hub's. |
| `RUNTIME_SECRET_KEY` | 32 bytes, hex or base64. Encrypts binding secrets, and later people's app tokens. Not in the database, so a stolen dump is ciphertext. |
| `RUNTIME_HUB_URL` | The hub. |
| `RUNTIME_PUBLIC_URL` | Where the hub posts. Must be HTTPS: the hub refuses private and loopback addresses, so a laptop uses socket mode instead. |
| `ANTHROPIC_API_KEY` | The model provider. One to begin with; pi-ai is what makes the rest cheap. |
| `RUNTIME_DEFAULT_MODEL` | What a template does not name. Default `claude-sonnet-5`. |
| `RUNTIME_DAILY_TOKEN_LIMIT` | Tokens one person may spend a day. Past it the agent says so and stops. 0 disables the ceiling — for a laptop, not a server. |
| `RUNTIME_PORT` | Default 8081. |

## What is here, and what is not

Built: the loop, streaming, typing, tool calls, the job queue with its retry
ladder, the agent registry with encrypted secrets, turns kept in full, the
weather tool and template, and the webhook door.

## Deploying

Its own EC2 instance, its own database, and a public HTTPS name.

```bash
RUNTIME_DEPLOY_HOST=admin@<ip> RUNTIME_DOMAIN=runtime.cuckoo.onl deploy/deploy-runtime.sh
deploy/deploy-runtime.sh rollback <tag>
```

**Why a public address for a service the hub could reach privately.** The hub
refuses to post webhooks to private, loopback or link-local addresses, so that
nobody can point a binding at a metadata service or a database on the network.
That rule applies to us too, deliberately: the moment there is an exception for
our own service, the protocol stops being the thing everyone uses. What
protects the open endpoint is the signature — `/hooks/{agentId}` verifies with
the secret of the agent the URL names before it reads the body.

Not yet, in the order `09` builds them: the catalogue and the `private` flag on
the hub, compaction and recall, the create flow in the app, connections and
Google Calendar, confirmations before writes, Gmail and Drive, the MCP client
and Swiggy, app webhooks, the subscription.

Never: a process, VM or sandbox per person; a filesystem or a shell for the
agent; anything inside the hub.
