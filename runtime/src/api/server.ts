/**
 * The two things this service exposes: whether it is alive, and the door the
 * hub posts events through.
 *
 * There is no API for people here. The app talks to the hub; the hub talks to
 * this. When the create flow arrives it will add endpoints the app calls, and
 * they will authenticate by asking the hub who the caller is — identity stays
 * the hub's.
 */

import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";

import { type InFlight, SignatureError, WebhookReceiver } from "@cuckoo/agent";

import type { Registry } from "../agents/registry.ts";
import type { Pool } from "../db/pool.ts";
import type { Jobs } from "../jobs.ts";

/** Bodies above this are not events; the hub's are a few kilobytes. */
const MAX_BODY_BYTES = 1 << 20;

export interface ServerDeps {
  pool: Pool;
  registry: Registry;
  jobs: Jobs;
  /** The runs in progress, so a stop can reach the one answering. */
  inflight?: InFlight;
  version: string;
  /** The hub's base URL, so health can say whether it is reachable. */
  hubUrl?: string;
}

export function createRuntimeServer(deps: ServerDeps): Server {
  // One receiver for every agent: it looks the secret up by the agent the URL
  // names, and verifies before it parses. Handlers are deliberately empty —
  // it verifies and the work is written down rather than done, because
  // answering inside the hub's ten-second budget is why the queue exists.
  const receiver = new WebhookReceiver({
    secretFor: (hubAgentId) => deps.registry.secretFor(hubAgentId),
    handlers: {},
  });

  return createServer((req, res) => {
    void route(req, res, deps, receiver).catch((error) => {
      console.error("runtime: unhandled request failure", error);
      send(res, 500, { error: "internal" });
    });
  });
}

async function route(
  req: IncomingMessage,
  res: ServerResponse,
  deps: ServerDeps,
  receiver: WebhookReceiver,
): Promise<void> {
  const url = new URL(req.url ?? "/", "http://runtime.local");

  if (req.method === "GET" && url.pathname === "/healthz") {
    return health(res, deps);
  }
  if (req.method === "POST" && url.pathname.startsWith("/hooks/")) {
    return hook(req, res, deps, receiver, url.pathname.slice("/hooks/".length));
  }
  send(res, 404, { error: "not_found" });
}

/**
 * Alive, and able to reach what it needs. Shaped like the hub's, so one thing
 * watches both.
 *
 * The hub is checked as well as the database, because a runtime that cannot
 * reach the hub cannot do the only thing it exists for — and reporting healthy
 * while silently answering nobody is the failure that goes unnoticed longest.
 * It is reported but does not fail the check: a hub that is briefly away is
 * not a reason to have this restarted underneath it.
 */
async function health(res: ServerResponse, deps: ServerDeps): Promise<void> {
  const [postgres, hub] = await Promise.all([
    deps.pool
      .query("SELECT 1")
      .then(() => "ok")
      .catch(() => "down"),
    reachable(deps.hubUrl),
  ]);

  send(res, postgres === "ok" ? 200 : 503, {
    status: postgres === "ok" ? (hub === "ok" ? "ok" : "degraded") : "down",
    version: deps.version,
    components: { postgres, hub },
  });
}

async function reachable(hubUrl?: string): Promise<string> {
  if (!hubUrl) return "unknown";
  try {
    const response = await fetch(`${hubUrl}/healthz`, { signal: AbortSignal.timeout(2_000) });
    return response.ok ? "ok" : "down";
  } catch {
    return "down";
  }
}

/**
 * One event from the hub.
 *
 * The agent is named by the URL, so the secret to verify with is chosen
 * before a byte of the body is believed. Then the job is written down and the
 * hub is answered — the model has not been called and will not be, here.
 */
async function hook(
  req: IncomingMessage,
  res: ServerResponse,
  deps: ServerDeps,
  receiver: WebhookReceiver,
  hubAgentId: string,
): Promise<void> {
  let body: Buffer;
  try {
    body = await readBody(req);
  } catch {
    send(res, 413, { error: "body_too_large" });
    return;
  }

  let envelope;
  try {
    ({ envelope } = await receiver.handle(hubAgentId, req.headers, body));
  } catch (error) {
    if (error instanceof SignatureError) {
      // Also what an agent this runtime does not serve gets: there is nothing
      // to verify against, and saying which of the two it was would tell a
      // stranger which agents exist here.
      send(res, 401, { error: "bad_signature" });
      return;
    }
    send(res, 422, { error: "bad_event" });
    return;
  }

  const agent = await deps.registry.find(hubAgentId);
  if (!agent) {
    send(res, 401, { error: "bad_signature" });
    return;
  }

  // A stop is acted on here, on arrival, not queued behind the very work it
  // is meant to stop: the run answering is cancelled, and whatever was still
  // waiting to be answered in that conversation is not.
  if (envelope.type === "stop.requested") {
    const conversation = (envelope.data.conversation ?? {}) as { id?: unknown };
    const conversationId = typeof conversation.id === "string" ? conversation.id : "";
    deps.inflight?.stop(conversationId);
    await deps.jobs.dropPending(agent.id, conversationId);
  }

  const fresh = await deps.jobs.enqueue({
    agentId: agent.id,
    eventId: envelope.id,
    type: envelope.type,
    payload: envelope.data,
  });
  // A redelivery is a success: the hub is told to stop sending it, and the
  // job it already has will be done, or has been.
  send(res, 200, { accepted: fresh });
}

async function readBody(req: IncomingMessage): Promise<Buffer> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of req) {
    const buffer = chunk as Buffer;
    size += buffer.length;
    if (size > MAX_BODY_BYTES) throw new Error("body too large");
    chunks.push(buffer);
  }
  return Buffer.concat(chunks);
}

function send(res: ServerResponse, status: number, body: unknown): void {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Content-Length": Buffer.byteLength(payload),
  });
  res.end(payload);
}
