/**
 * A conversation, and a reply being written into one.
 *
 * These are what a handler is handed: `conv.send("…")` rather than
 * `client.send(conv.id, "…")`, so the id is never the thing you got wrong.
 */

import type { ActivityState, HubClient, SendOptions } from "./client.ts";
import { StoppedError } from "./errors.ts";
import type { Buttons, Message, Participant } from "./models.ts";

/** An activity lasts ten seconds on the hub; it is said again well before. */
const RENEW_MS = 7_000;

/** A signal that never fires, for a conversation nobody can stop. */
const NEVER = new AbortController().signal;

/** Where a message was said, and the handle for replying there. */
export class Conversation {
  readonly id: string;
  readonly kind: string;
  readonly participants: Participant[];
  /**
   * Fires when the person presses stop while your handler is working on this
   * conversation. Hand it to anything that can be cancelled — a model call, a
   * `fetch` — and the work ends with the words. Ignore it and nothing breaks:
   * every write from here throws `StoppedError` once it has fired, so the
   * handler ends at its next write, and the SDK treats that as done.
   */
  readonly signal: AbortSignal;
  private readonly client: HubClient;

  constructor(id: string, kind: string, participants: Participant[], client: HubClient, signal: AbortSignal = NEVER) {
    this.id = id;
    this.kind = kind;
    this.participants = participants;
    this.client = client;
    this.signal = signal;
  }

  /** Whether the person has stopped this handler's work. */
  get stopped(): boolean {
    return this.signal.aborted;
  }

  /** Say something here. */
  async send(text = "", options: SendOptions = {}): Promise<Message> {
    this.guard();
    return this.client.send(this.id, text, options);
  }

  /**
   * Begin a reply that arrives piece by piece:
   *
   * ```ts
   * const reply = await conv.stream();
   * for await (const token of model) await reply.append(token);
   * await reply.finish();
   * ```
   *
   * Buttons and quick replies are attached when it finishes, not at the start.
   * While it streams the person sees "writing…" and a stop button.
   */
  async stream(options: { replyTo?: string } = {}): Promise<Stream> {
    this.guard();
    const started = await this.client.startStream(this.id, options);
    return new Stream(this.client, this.id, started.id, this.signal);
  }

  /**
   * Keep the person told what you are doing while `work` runs:
   *
   * ```ts
   * const forecast = await conv.working("Checking the weather", () => weather(city));
   * ```
   *
   * The label is shown under the agent's name, in the chat and in the chat
   * list — one line, at most 40 characters, no links. It is renewed while the
   * work runs and cleared when it ends, however it ends. If the person
   * presses stop, this rejects with `StoppedError` at once rather than
   * waiting for `work`.
   */
  working<T>(label: string, work: () => Promise<T> | T): Promise<T> {
    return this.busy("working", label, work);
  }

  /** `working` with nothing to name: the person sees "thinking…". */
  thinking<T>(work: () => Promise<T> | T): Promise<T> {
    return this.busy("thinking", undefined, work);
  }

  /** Say once what you are doing; it shows for ten seconds. */
  async activity(state: ActivityState, label?: string): Promise<void> {
    this.guard();
    return this.client.activity(this.id, state, label);
  }

  /** Show, or hide, the "working" indicator. The first version of `activity`. */
  async typing(state: "start" | "stop" = "start"): Promise<void> {
    this.guard();
    return this.client.typing(this.id, state);
  }

  /** Recent messages here, newest last. */
  history(options: { limit?: number; before?: string } = {}): Promise<Message[]> {
    return this.client.history(this.id, options);
  }

  /** The person in a DM, or undefined if there is no human in it. */
  get person(): Participant | undefined {
    return this.participants.find((p) => p.kind === "user");
  }

  private guard(): void {
    if (this.signal.aborted) throw new StoppedError();
  }

  private async busy<T>(state: ActivityState, label: string | undefined, work: () => Promise<T> | T): Promise<T> {
    this.guard();
    // Saying what you are doing is a courtesy, never a reason to fail.
    const say = () => this.client.activity(this.id, state, label).catch(() => {});
    await say();
    const timer = setInterval(() => void say(), RENEW_MS);
    let onAbort: (() => void) | undefined;
    const stopped = new Promise<never>((_, reject) => {
      onAbort = () => reject(new StoppedError());
      this.signal.addEventListener("abort", onAbort, { once: true });
    });
    try {
      return await Promise.race([Promise.resolve().then(work), stopped]);
    } finally {
      clearInterval(timer);
      if (onAbort) this.signal.removeEventListener("abort", onAbort);
      // After a stop the hub has already cleared it on every screen.
      if (!this.signal.aborted) await this.client.activity(this.id, "idle").catch(() => {});
    }
  }
}

/**
 * A message being written. `append` adds text; `finish` seals it and attaches
 * whatever buttons it ends with.
 *
 * Finishing twice is not an error here — the second call does nothing — so a
 * `finally` that finishes a stream is safe next to one that already did.
 */
export class Stream {
  readonly conversationId: string;
  readonly messageId: string;
  private readonly client: HubClient;
  private readonly signal: AbortSignal;
  private finished = false;

  constructor(client: HubClient, conversationId: string, messageId: string, signal: AbortSignal = NEVER) {
    this.client = client;
    this.conversationId = conversationId;
    this.messageId = messageId;
    this.signal = signal;
  }

  /**
   * Add text to the message. Throws `StoppedError` once the person has
   * pressed stop: the hub has already ended the message where it stood.
   */
  async append(text: string): Promise<void> {
    if (this.finished) throw new Error("this stream is already finished");
    if (this.signal.aborted) throw new StoppedError();
    if (!text) return;
    await this.client.appendStream(this.messageId, text);
  }

  /** Seal the message. After a stop this returns the message as the person left it. */
  async finish(options: { buttons?: Buttons; quickReplies?: string[] } = {}): Promise<Message | undefined> {
    if (this.finished) return undefined;
    this.finished = true;
    return this.client.finishStream(this.messageId, this.conversationId, options);
  }

  get isFinished(): boolean {
    return this.finished;
  }
}
