/**
 * HubClient: everything one agent can ask the hub to do, over HTTP.
 *
 * This is the whole protocol surface except receiving. Both ways of receiving
 * — a socket the backend holds open, and webhooks the hub posts — send with
 * this, so a handler is written once and runs under either.
 */

import { ProtocolError, raiseForStatus } from "./errors.ts";
import {
  DEFAULT_HUB,
  type Attachment,
  type Buttons,
  type Message,
  buttonsToWire,
  parseAttachment,
  parseMessage,
  quickRepliesToWire,
} from "./models.ts";

/** What a message can carry besides its text. */
/** What an agent can say it is doing. */
export type ActivityState = "thinking" | "working" | "idle";

export interface SendOptions {
  attachments?: (string | Attachment)[];
  buttons?: Buttons;
  quickReplies?: string[];
  replyTo?: string;
  /** A retry with the same key returns the same message instead of a second one. */
  idempotencyKey?: string;
}

export interface ClientOptions {
  /** The hub's base URL. `http://localhost:8080` for a local one. */
  hub?: string;
  /** How long to wait for the hub to answer one request. */
  timeoutMs?: number;
  /** Swapped out in tests. Defaults to the global `fetch`. */
  fetch?: typeof fetch;
}

const SECRET_PREFIX = "bnd_sec_";

export class HubClient {
  readonly hub: string;
  private readonly secret: string;
  private readonly timeoutMs: number;
  private readonly doFetch: typeof fetch;

  /**
   * `secret` is the binding secret shown once when the binding was set. It is
   * the agent's whole identity: whoever holds it speaks for the agent.
   */
  constructor(secret: string, options: ClientOptions = {}) {
    if (!secret.startsWith(SECRET_PREFIX)) {
      throw new Error(`secret must be a binding secret, which starts with ${SECRET_PREFIX}`);
    }
    this.secret = secret;
    this.hub = (options.hub ?? DEFAULT_HUB).replace(/\/+$/, "");
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.doFetch = options.fetch ?? globalThis.fetch;
  }

  /** The Authorization header every call carries. Also used by the socket. */
  authHeaders(): Record<string, string> {
    return { Authorization: `Bearer ${this.secret}` };
  }

  // -- sending -------------------------------------------------------------

  /** Say something in a conversation the agent is in. */
  async send(conversationId: string, text = "", options: SendOptions = {}): Promise<Message> {
    const body: Record<string, unknown> = {
      text,
      idempotency_key: options.idempotencyKey ?? crypto.randomUUID(),
    };
    const mediaIds = mediaIdsOf(options.attachments);
    if (mediaIds.length) body.attachments = mediaIds;
    if (options.replyTo) body.reply_to = options.replyTo;
    const buttons = buttonsToWire(options.buttons);
    if (buttons) body.buttons = buttons;
    const quickReplies = quickRepliesToWire(options.quickReplies);
    if (quickReplies) body.quick_replies = quickReplies;

    const data = await this.json("POST", `/v1/agent/conversations/${conversationId}/messages`, body);
    return parseMessage(data.message, conversationId);
  }

  /** Show, or hide, the "working" indicator on the person's device. */
  async typing(conversationId: string, state: "start" | "stop" = "start"): Promise<void> {
    await this.call("POST", `/v1/agent/conversations/${conversationId}/typing`, { state });
  }

  /**
   * Say what the agent is doing: `thinking`, `working` with a short label
   * the person sees ("Checking the weather"), or `idle`. It shows for ten
   * seconds unless said again; `Conversation.working` keeps it up for you.
   */
  async activity(conversationId: string, state: ActivityState, label?: string): Promise<void> {
    const body: Record<string, unknown> = { state };
    if (label) body.label = label;
    await this.call("POST", `/v1/agent/conversations/${conversationId}/activity`, body);
  }

  // -- streaming -----------------------------------------------------------

  /**
   * Begin a message that arrives piece by piece. Returns the id to append to.
   * A start that carries text, buttons or files is refused by the hub.
   */
  async startStream(conversationId: string, options: { replyTo?: string } = {}): Promise<Message> {
    const body: Record<string, unknown> = { stream: true };
    if (options.replyTo) body.reply_to = options.replyTo;
    const data = await this.json("POST", `/v1/agent/conversations/${conversationId}/messages`, body);
    return parseMessage(data.message, conversationId);
  }

  /**
   * Add text to a stream. The hub finishes a stream left idle for 30 seconds
   * and marks it truncated, so keep appending or finish.
   */
  async appendStream(messageId: string, text: string): Promise<void> {
    await this.call("POST", `/v1/agent/messages/${messageId}/append`, { text });
  }

  /** Finish a stream, attaching buttons and quick replies if there are any. */
  async finishStream(
    messageId: string,
    conversationId: string,
    options: { buttons?: Buttons; quickReplies?: string[] } = {},
  ): Promise<Message> {
    const body: Record<string, unknown> = {};
    const buttons = buttonsToWire(options.buttons);
    if (buttons) body.buttons = buttons;
    const quickReplies = quickRepliesToWire(options.quickReplies);
    if (quickReplies) body.quick_replies = quickReplies;
    const data = await this.json("POST", `/v1/agent/messages/${messageId}/finish`, body);
    return parseMessage(data.message, conversationId);
  }

  // -- reading -------------------------------------------------------------

  /** Who this secret speaks for. */
  async me(): Promise<Record<string, unknown>> {
    return this.json("GET", "/v1/agent/me");
  }

  /**
   * What the agent missed. A backend that lost its state, or one that polls
   * instead of receiving, catches up here.
   */
  async events(options: { since?: string; limit?: number } = {}): Promise<Record<string, unknown>[]> {
    const query = new URLSearchParams();
    if (options.since) query.set("since", options.since);
    if (options.limit) query.set("limit", String(options.limit));
    const data = await this.json("GET", `/v1/agent/events?${query}`);
    return Array.isArray(data.events) ? (data.events as Record<string, unknown>[]) : [];
  }

  /** Recent messages of one conversation, newest last. */
  async history(
    conversationId: string,
    options: { limit?: number; before?: string } = {},
  ): Promise<Message[]> {
    const query = new URLSearchParams();
    if (options.limit) query.set("limit", String(options.limit));
    if (options.before) query.set("before", options.before);
    const data = await this.json(
      "GET",
      `/v1/agent/conversations/${conversationId}/messages?${query}`,
    );
    const messages = Array.isArray(data.messages) ? data.messages : [];
    return messages.map((m) => parseMessage(m, conversationId));
  }

  // -- files ---------------------------------------------------------------

  /**
   * Put a file on the hub, ready to be sent. Sending is a separate step, so a
   * slow upload does not hold a message open; an upload nobody sends is
   * removed after a day.
   */
  async upload(
    fileName: string,
    bytes: Uint8Array | Blob,
    options: { contentType?: string; durationMs?: number; waveform?: number[]; audio?: boolean } = {},
  ): Promise<Attachment> {
    const contentType = options.contentType ?? "application/octet-stream";
    const query = new URLSearchParams();
    if (options.durationMs) query.set("duration_ms", String(Math.trunc(options.durationMs)));
    if (options.waveform?.length) {
      query.set("waveform", options.waveform.map((n) => Math.trunc(n)).join(","));
    }
    if (options.audio || contentType.startsWith("audio/")) query.set("kind", "audio");

    const form = new FormData();
    const blob = bytes instanceof Blob ? bytes : new Blob([bytes as BlobPart], { type: contentType });
    form.append("file", blob, fileName);

    // No timeout: a big file on a slow line is not a stuck request.
    const response = await this.doFetch(`${this.hub}/v1/agent/media?${query}`, {
      method: "POST",
      headers: this.authHeaders(),
      body: form,
    });
    await raiseForStatus(response);
    const data = (await response.json()) as { media?: unknown };
    return parseAttachment(data.media);
  }

  /** Fetch the bytes of a file on a message in one of this agent's conversations. */
  async download(mediaId: string, options: { thumbnail?: boolean } = {}): Promise<Uint8Array> {
    const query = options.thumbnail ? "?variant=thumb" : "";
    const response = await this.doFetch(`${this.hub}/v1/agent/media/${mediaId}${query}`, {
      headers: this.authHeaders(),
    });
    await raiseForStatus(response);
    return new Uint8Array(await response.arrayBuffer());
  }

  // -- plumbing ------------------------------------------------------------

  private async call(method: string, path: string, body?: unknown): Promise<Response> {
    const response = await this.doFetch(`${this.hub}${path}`, {
      method,
      headers: {
        ...this.authHeaders(),
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      signal: AbortSignal.timeout(this.timeoutMs),
    });
    await raiseForStatus(response);
    return response;
  }

  private async json(method: string, path: string, body?: unknown): Promise<Record<string, unknown>> {
    const response = await this.call(method, path, body);
    if (response.status === 204) return {};
    const text = await response.text();
    if (!text) return {};
    try {
      return JSON.parse(text) as Record<string, unknown>;
    } catch {
      throw new ProtocolError("bad_response", `the hub answered ${path} with something that is not JSON`);
    }
  }
}

/** Turn what a caller passed into media ids. Uploading a path is the caller's job. */
function mediaIdsOf(attachments?: (string | Attachment)[]): string[] {
  if (!attachments?.length) return [];
  return attachments.map((item) => (typeof item === "string" ? item : item.mediaId));
}
