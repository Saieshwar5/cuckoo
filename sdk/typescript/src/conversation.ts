/**
 * A conversation, and a reply being written into one.
 *
 * These are what a handler is handed: `conv.send("…")` rather than
 * `client.send(conv.id, "…")`, so the id is never the thing you got wrong.
 */

import type { HubClient, SendOptions } from "./client.ts";
import type { Buttons, Message, Participant } from "./models.ts";

/** Where a message was said, and the handle for replying there. */
export class Conversation {
  readonly id: string;
  readonly kind: string;
  readonly participants: Participant[];
  private readonly client: HubClient;

  constructor(id: string, kind: string, participants: Participant[], client: HubClient) {
    this.id = id;
    this.kind = kind;
    this.participants = participants;
    this.client = client;
  }

  /** Say something here. */
  send(text = "", options: SendOptions = {}): Promise<Message> {
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
   */
  async stream(options: { replyTo?: string } = {}): Promise<Stream> {
    const started = await this.client.startStream(this.id, options);
    return new Stream(this.client, this.id, started.id);
  }

  /** Show, or hide, the "working" indicator. A stream shows it by itself. */
  typing(state: "start" | "stop" = "start"): Promise<void> {
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
  private finished = false;

  constructor(client: HubClient, conversationId: string, messageId: string) {
    this.client = client;
    this.conversationId = conversationId;
    this.messageId = messageId;
  }

  /** Add text to the message. */
  async append(text: string): Promise<void> {
    if (this.finished) throw new Error("this stream is already finished");
    if (!text) return;
    await this.client.appendStream(this.messageId, text);
  }

  /** Seal the message. */
  async finish(options: { buttons?: Buttons; quickReplies?: string[] } = {}): Promise<Message | undefined> {
    if (this.finished) return undefined;
    this.finished = true;
    return this.client.finishStream(this.messageId, this.conversationId, options);
  }

  get isFinished(): boolean {
    return this.finished;
  }
}
