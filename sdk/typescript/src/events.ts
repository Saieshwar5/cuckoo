/**
 * Events: the envelope every one arrives in, and the routing that turns one
 * into a call to your handler.
 *
 * Both ways of receiving share this file. A socket reads frames and a webhook
 * reads request bodies, but what arrives is the same envelope and is handed to
 * the same handlers — which is why a backend can move between the two without
 * touching the code that answers.
 */

import type { HubClient } from "./client.ts";
import { Conversation } from "./conversation.ts";
import { StoppedError } from "./errors.ts";
import { parseSchedule, type ScheduleChange } from "./schedules.ts";
import { type Message, type PairToken, parseMessage, parseParticipant, parsePairToken } from "./models.ts";

export const EVENT_MESSAGE_CREATED = "message.created";
export const EVENT_CONVERSATION_JOINED = "conversation.joined";
export const EVENT_CONVERSATION_LEFT = "conversation.left";
export const EVENT_STOP_REQUESTED = "stop.requested";
export const EVENT_SCHEDULE_REQUESTED = "schedule.requested";
export const EVENT_SCHEDULE_UPDATED = "schedule.updated";
export const EVENT_SCHEDULE_DELETED = "schedule.deleted";

/**
 * The envelope around every event. `id` is what you acknowledge and
 * de-duplicate on: a retry carries the same one.
 */
export interface Envelope {
  id: string;
  type: string;
  createdAt: string;
  agentId: string;
  data: Record<string, unknown>;
}

export type MessageHandler = (message: Message, conversation: Conversation) => void | Promise<void>;
export type JoinHandler = (conversation: Conversation, token?: PairToken) => void | Promise<void>;
export type LeaveHandler = (conversationId: string, reason: string) => void | Promise<void>;

/** A person pressing stop. `messageId` is the reply the hub ended, if one was being written. */
export interface StopRequest {
  conversationId: string;
  messageId?: string;
}
export type StopHandler = (stop: StopRequest, conversation: Conversation) => void | Promise<void>;
export type ScheduleHandler = (change: ScheduleChange, conversation: Conversation) => void | Promise<void>;

export interface Handlers {
  /** Every message the agent receives. The one that matters. */
  onMessage?: MessageHandler;
  /** Someone added the agent: a new conversation exists. The place to say hello. */
  onJoin?: JoinHandler;
  /** The agent was removed or blocked. Stop trying, rather than collecting 403s. */
  onLeave?: LeaveHandler;
  /**
   * The person pressed stop. Handlers still running for this conversation in
   * this process are cancelled before this runs; what is left is to say
   * something true — "Stopped. Nothing was booked." — if there is anything
   * worth saying.
   */
  onStop?: StopHandler;
  /**
   * The person made, changed or deleted a schedule in the app. Hold it in
   * your own timer and confirm it; the hub only shows it.
   */
  onSchedule?: ScheduleHandler;
}

/**
 * The message handlers running right now, by conversation, so a stop can
 * reach them. Only this process's: a stop that lands on another process is
 * felt here at the next write, which the hub refuses as `stopped`.
 */
export class InFlight {
  private readonly running = new Map<string, Set<AbortController>>();

  start(conversationId: string): AbortController {
    const controller = new AbortController();
    const set = this.running.get(conversationId) ?? new Set();
    set.add(controller);
    this.running.set(conversationId, set);
    return controller;
  }

  end(conversationId: string, controller: AbortController): void {
    const set = this.running.get(conversationId);
    if (!set) return;
    set.delete(controller);
    if (!set.size) this.running.delete(conversationId);
  }

  /** Cancel everything running for a conversation. Returns how much there was. */
  stop(conversationId: string): number {
    const set = this.running.get(conversationId);
    if (!set) return 0;
    for (const controller of set) controller.abort(new StoppedError());
    this.running.delete(conversationId);
    return set.size;
  }
}

/** Read an envelope, or throw if it is not one. */
export function parseEnvelope(raw: unknown): Envelope {
  const w = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  const id = typeof w.id === "string" ? w.id : "";
  const type = typeof w.type === "string" ? w.type : "";
  if (!id || !type) throw new Error("not an event: it has no id or no type");
  return {
    id,
    type,
    createdAt: typeof w.created_at === "string" ? w.created_at : "",
    agentId: typeof w.agent_id === "string" ? w.agent_id : "",
    data: (w.data && typeof w.data === "object" ? w.data : {}) as Record<string, unknown>,
  };
}

function conversationOf(data: Record<string, unknown>, client: HubClient, signal?: AbortSignal): Conversation {
  const conv = (data.conversation ?? {}) as Record<string, unknown>;
  const participants = Array.isArray(data.participants) ? data.participants : [];
  return new Conversation(
    typeof conv.id === "string" ? conv.id : "",
    typeof conv.kind === "string" ? conv.kind : "",
    participants.map(parseParticipant),
    client,
    signal,
  );
}

function conversationIdOf(data: Record<string, unknown>): string {
  const conv = (data.conversation ?? {}) as Record<string, unknown>;
  return typeof conv.id === "string" ? conv.id : "";
}

/**
 * Route one event to the handler that wants it.
 *
 * Anything the handlers do not cover is a success: an event type added to the
 * protocol later must not stop a backend written before it existed. So is a
 * handler ended by the person pressing stop: its work is exactly what they
 * did not want done again.
 */
export async function dispatch(
  envelope: Envelope,
  client: HubClient,
  handlers: Handlers,
  inflight?: InFlight,
): Promise<void> {
  switch (envelope.type) {
    case EVENT_MESSAGE_CREATED: {
      if (!handlers.onMessage) return;
      const id = conversationIdOf(envelope.data);
      const controller = inflight?.start(id);
      const conversation = conversationOf(envelope.data, client, controller?.signal);
      const message = parseMessage(envelope.data.message, conversation.id, envelope.id);
      try {
        await handlers.onMessage(message, conversation);
      } catch (error) {
        if (error instanceof StoppedError) return;
        throw error;
      } finally {
        if (controller) inflight?.end(id, controller);
      }
      return;
    }
    case EVENT_STOP_REQUESTED: {
      const id = conversationIdOf(envelope.data);
      inflight?.stop(id);
      if (!handlers.onStop) return;
      const messageId = typeof envelope.data.message_id === "string" ? envelope.data.message_id : undefined;
      await handlers.onStop({ conversationId: id, messageId }, conversationOf(envelope.data, client));
      return;
    }
    case EVENT_CONVERSATION_JOINED: {
      if (!handlers.onJoin) return;
      const conversation = conversationOf(envelope.data, client);
      await handlers.onJoin(conversation, parsePairToken(envelope.data.pair_token));
      return;
    }
    case EVENT_CONVERSATION_LEFT: {
      if (!handlers.onLeave) return;
      const conv = (envelope.data.conversation ?? {}) as Record<string, unknown>;
      const reason = typeof envelope.data.reason === "string" ? envelope.data.reason : "";
      await handlers.onLeave(typeof conv.id === "string" ? conv.id : "", reason);
      return;
    }
    case EVENT_SCHEDULE_REQUESTED:
    case EVENT_SCHEDULE_UPDATED:
    case EVENT_SCHEDULE_DELETED: {
      if (!handlers.onSchedule) return;
      const type = envelope.type.slice("schedule.".length) as ScheduleChange["type"];
      await handlers.onSchedule(
        { type, schedule: parseSchedule(envelope.data.schedule) },
        conversationOf(envelope.data, client),
      );
      return;
    }
    default:
      return;
  }
}

/**
 * The event ids seen recently, so one delivered twice is answered once.
 *
 * The hub redelivers anything it never saw acknowledged, so a slow handler or
 * a dropped connection means seeing an event again. Bounded, because a backend
 * that runs for a year must not grow for a year.
 */
export class SeenEvents {
  private readonly seen = new Set<string>();
  private readonly limit: number;

  constructor(limit = 1000) {
    this.limit = limit;
  }

  /** True if this id has been seen before. Records it either way. */
  check(id: string): boolean {
    if (this.seen.has(id)) return true;
    this.seen.add(id);
    // A Set keeps insertion order, so the oldest is the first key.
    while (this.seen.size > this.limit) {
      const oldest = this.seen.values().next().value;
      if (oldest === undefined) break;
      this.seen.delete(oldest);
    }
    return false;
  }
}
