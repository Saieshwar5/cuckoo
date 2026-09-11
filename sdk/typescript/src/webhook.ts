/**
 * Receiving over webhooks: verifying what the hub posted, and answering it.
 *
 * This is what a backend serving many agents from one address uses. A socket
 * is one connection per agent, which is fine for one agent and impossible for
 * ten thousand; a webhook is one URL that serves all of them, and the agent an
 * event belongs to is named by the URL it was posted to.
 *
 * The order here is the security of it: verify with the secret of the agent
 * the URL names, and only then believe anything inside the body.
 */

import { createHash, createHmac, timingSafeEqual } from "node:crypto";

import { HubClient, type ClientOptions } from "./client.ts";
import { type Envelope, type Handlers, InFlight, dispatch, parseEnvelope } from "./events.ts";

/** The headers the hub sends with every webhook. */
export const HEADER_EVENT = "x-cuckoo-event";
export const HEADER_EVENT_ID = "x-cuckoo-event-id";
export const HEADER_TIMESTAMP = "x-cuckoo-timestamp";
export const HEADER_SIGNATURE = "x-cuckoo-signature";

/** How far from now a timestamp may be before the request is a replay. */
const DEFAULT_TOLERANCE_SECONDS = 300;

/**
 * The key that signs webhooks for a binding: the SHA-256 of the binding
 * secret, not the secret itself.
 *
 * The hub stores that hash to authenticate the backend, so signing with it
 * means the secret's plaintext is never at rest on the hub, and a backend
 * derives the same key from the secret it was handed.
 */
export function signingKey(secret: string): Buffer {
  return createHash("sha256").update(secret, "utf8").digest();
}

/** The value of the signature header for a body: `sha256=<hex>`. */
export function sign(key: Buffer, timestamp: string, body: string | Buffer): string {
  const mac = createHmac("sha256", key);
  mac.update(timestamp);
  mac.update(".");
  mac.update(body);
  return `sha256=${mac.digest("hex")}`;
}

export class SignatureError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SignatureError";
  }
}

/**
 * Check that a body came from the hub, unaltered, recently.
 *
 * `body` must be the bytes as they arrived. A body parsed into an object and
 * re-encoded is a different string — key order, spacing — and will not verify.
 */
export function verify(
  secret: string,
  headers: Headers | Record<string, string | string[] | undefined>,
  body: string | Buffer,
  options: { toleranceSeconds?: number; now?: number } = {},
): void {
  const timestamp = headerOf(headers, HEADER_TIMESTAMP);
  const signature = headerOf(headers, HEADER_SIGNATURE);
  if (!timestamp || !signature) throw new SignatureError("the request carries no signature");

  const seconds = Number(timestamp);
  if (!Number.isFinite(seconds)) throw new SignatureError("the timestamp is not a number");
  const now = options.now ?? Math.floor(Date.now() / 1000);
  const tolerance = options.toleranceSeconds ?? DEFAULT_TOLERANCE_SECONDS;
  if (Math.abs(now - seconds) > tolerance) {
    throw new SignatureError("the timestamp is too far from now; this may be a replay");
  }

  const expected = Buffer.from(sign(signingKey(secret), timestamp, body), "utf8");
  const given = Buffer.from(signature, "utf8");
  // Compare in constant time, and only when the lengths match — timingSafeEqual
  // throws on a length mismatch, which would itself leak the length.
  if (expected.length !== given.length || !timingSafeEqual(expected, given)) {
    throw new SignatureError("the signature does not match");
  }
}

function headerOf(
  headers: Headers | Record<string, string | string[] | undefined>,
  name: string,
): string {
  if (typeof (headers as Headers).get === "function") {
    return (headers as Headers).get(name) ?? "";
  }
  const record = headers as Record<string, string | string[] | undefined>;
  const value = record[name] ?? record[name.toLowerCase()] ?? record[name.toUpperCase()];
  if (Array.isArray(value)) return value[0] ?? "";
  return value ?? "";
}

/** How the receiver finds the secret of the agent an event was posted for. */
export type SecretLookup = (agentId: string) => Promise<string | undefined> | string | undefined;

export interface ReceiverOptions extends ClientOptions {
  /**
   * The binding secret for an agent, or undefined if this backend does not
   * serve it. A backend with one agent returns a constant; one serving
   * thousands reads its database.
   */
  secretFor: SecretLookup;
  handlers: Handlers;
  /** Seconds a timestamp may differ from now. Default 300. */
  toleranceSeconds?: number;
}

export interface ReceivedEvent {
  envelope: Envelope;
  client: HubClient;
}

/**
 * A webhook receiver for any number of agents.
 *
 * `handle` does the verifying and the routing and nothing else — it does not
 * listen on a port, because a backend already has a server and wants this
 * mounted inside it.
 *
 * **Answer the hub before doing the work.** The hub gives a backend ten
 * seconds and retries anything slower, so a handler that calls a model must
 * write the event down, answer 200, and answer the person afterwards. The
 * `defer` option does exactly this: handlers run after the promise resolves.
 */
export class WebhookReceiver {
  private readonly options: ReceiverOptions;
  // Handlers running in this process, so a stop posted while one runs
  // cancels it.
  private readonly inflight = new InFlight();

  constructor(options: ReceiverOptions) {
    this.options = options;
  }

  /**
   * Verify one request and route it. Throws `SignatureError` if it did not
   * come from the hub — answer that with 401 and nothing else.
   *
   * `agentId` comes from the URL the hub posted to, so the secret is chosen
   * before the body is read. The envelope's own `agent_id` is then checked
   * against it: a body that disagrees with the URL it arrived at is refused.
   */
  async handle(agentId: string, headers: Headers | Record<string, string | string[] | undefined>, body: string | Buffer): Promise<ReceivedEvent> {
    const secret = await this.options.secretFor(agentId);
    if (!secret) throw new SignatureError(`no secret for ${agentId}`);

    verify(secret, headers, body, { toleranceSeconds: this.options.toleranceSeconds });

    const envelope = parseEnvelope(JSON.parse(body.toString()));
    if (envelope.agentId && envelope.agentId !== agentId) {
      throw new SignatureError("the event names a different agent than the URL it arrived at");
    }

    const client = new HubClient(secret, this.options);
    await dispatch(envelope, client, this.options.handlers, this.inflight);
    return { envelope, client };
  }
}
