/**
 * Agent: a backend for one agent identity, over a socket.
 *
 * ```ts
 * const agent = new Agent({ secret: process.env.CUCKOO_SECRET!, hub: "http://localhost:8080" });
 * agent.onMessage(async (msg, conv) => { await conv.send(`you said: ${msg.text}`); });
 * await agent.serve();
 * ```
 *
 * A backend answering for many agents at once does not use this class: it
 * mounts a `WebhookReceiver` and holds no connections at all.
 */

import { HubClient, type ClientOptions } from "./client.ts";
import {
  type Handlers,
  InFlight,
  type JoinHandler,
  type LeaveHandler,
  type MessageHandler,
  type ScheduleHandler,
  SeenEvents,
  type StopHandler,
  dispatch,
  parseEnvelope,
} from "./events.ts";
import { CLOSE_BINDING_GONE, CLOSE_REPLACED, SocketSession } from "./socket.ts";

export interface AgentOptions extends ClientOptions {
  secret: string;
  /** Longest gap between reconnection attempts. Default 30s. */
  maxBackoffMs?: number;
  /** Swapped out in tests. */
  WebSocketImpl?: typeof WebSocket;
}

export class Agent {
  readonly client: HubClient;
  private readonly options: AgentOptions;
  private readonly handlers: Handlers = {};
  private readonly seen = new SeenEvents();
  private readonly inflight = new InFlight();
  private session?: SocketSession;
  private stopped = false;

  constructor(options: AgentOptions) {
    this.options = options;
    this.client = new HubClient(options.secret, options);
  }

  /** Register what runs for every message. */
  onMessage(fn: MessageHandler): this {
    this.handlers.onMessage = fn;
    return this;
  }

  /** Register what runs when someone adds the agent. The place to say hello. */
  onJoin(fn: JoinHandler): this {
    this.handlers.onJoin = fn;
    return this;
  }

  /** Register what runs when the agent is removed or blocked. */
  onLeave(fn: LeaveHandler): this {
    this.handlers.onLeave = fn;
    return this;
  }

  /**
   * Register what runs when the person presses stop. Without one, stopping
   * still works: the handler for that conversation is cancelled, and its
   * next write throws `StoppedError`, which ends it quietly.
   */
  onStop(fn: StopHandler): this {
    this.handlers.onStop = fn;
    return this;
  }

  /**
   * Register what runs when the person makes, changes or deletes a schedule
   * in the app. Hold it in your own timer, then `conversation.schedules
   * .confirm(schedule.id)`; the app says "waiting" until you do.
   */
  onSchedule(fn: ScheduleHandler): this {
    this.handlers.onSchedule = fn;
    return this;
  }

  /**
   * Connect and serve, reconnecting when the connection drops, until `stop()`
   * or until the hub says not to come back.
   */
  async serve(): Promise<void> {
    if (!this.handlers.onMessage) {
      throw new Error("register a handler with onMessage before serving");
    }
    const maxBackoff = this.options.maxBackoffMs ?? 30_000;
    let backoff = 1_000;

    while (!this.stopped) {
      const startedAt = Date.now();
      const { code } = await this.session_();

      if (this.stopped) return;
      if (code === CLOSE_REPLACED || code === CLOSE_BINDING_GONE) {
        // Another connection took over, or the binding is gone. Coming back
        // would fight with whatever replaced us.
        return;
      }
      // A connection that lasted a while earned a fresh backoff.
      if (Date.now() - startedAt > 60_000) backoff = 1_000;
      await sleep(backoff);
      backoff = Math.min(backoff * 2, maxBackoff);
    }
  }

  /** Stop serving. The current connection is closed. */
  stop(): void {
    this.stopped = true;
    this.session?.close();
  }

  private async session_(): Promise<{ code: number; reason: string }> {
    const url = `${this.client.hub.replace(/^http/, "ws")}/v1/agent/socket`;
    const session = new SocketSession({
      url,
      headers: this.client.authHeaders(),
      WebSocketImpl: this.options.WebSocketImpl,
      onEvent: (raw) => void this.handle(session, raw),
    });
    this.session = session;
    try {
      return await session.open();
    } catch {
      return { code: 0, reason: "could not connect" };
    }
  }

  /**
   * Handle one event, then acknowledge it.
   *
   * A handler that throws is not acknowledged, so the hub sends it again —
   * which is what you want for a backend that was briefly unable to answer,
   * and what you must guard against for one that will never succeed.
   */
  private async handle(session: SocketSession, raw: unknown): Promise<void> {
    let envelope;
    try {
      envelope = parseEnvelope(raw);
    } catch {
      return;
    }
    if (this.seen.check(envelope.id)) {
      session.ack(envelope.id);
      return;
    }
    try {
      await dispatch(envelope, this.client, this.handlers, this.inflight);
    } catch (error) {
      console.error(`cuckoo: handler failed for ${envelope.id}; the hub will send it again`, error);
      return;
    }
    session.ack(envelope.id);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
