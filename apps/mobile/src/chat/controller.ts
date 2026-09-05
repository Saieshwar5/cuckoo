import { newIdempotencyKey, type Api } from '../api/client';
import type { Message } from '../api/types';
import type { Realtime } from '../realtime/realtime';
import {
  addLocal,
  addOlder,
  applyFrame,
  confirmLocal,
  empty,
  failLocal,
  isTyping,
  newestServerId,
  quickReplies,
  retryLocal,
  setPage,
  upsert,
  type ChatMessage,
  type ChatState,
} from './store';

export interface ChatSnapshot {
  messages: ChatMessage[];
  typing: boolean;
  loading: boolean;
  loadingOlder: boolean;
  hasOlder: boolean;
  connected: boolean;
  error: unknown;
  quickReplies: string[];
}

// What a screen asks to send: words, or a tap on a button an agent
// offered, either quoting an earlier message.
export interface SendRequest {
  text?: string;
  action?: { button_id: string; source_message_id: string; label: string };
  replyTo?: Message;
}

const PAGE = 50;

// ChatController is one open conversation as a thing outside React: its
// history loaded from the hub, kept live by the session's connection, our
// own sends shown before the hub answers and retried with the same key.
export class ChatController {
  private state: ChatState = empty;
  private snapshot: ChatSnapshot = {
    messages: [],
    typing: false,
    loading: true,
    loadingOlder: false,
    hasOlder: false,
    connected: false,
    error: null,
    quickReplies: [],
  };
  private listeners = new Set<() => void>();
  private unsubscribe: (() => void) | null = null;
  private typingTimer: ReturnType<typeof setTimeout> | null = null;
  private pending = new Map<string, SendRequest>();
  private stopped = false;

  constructor(
    private readonly api: Api,
    private readonly realtime: Realtime,
    private readonly conversationId: string,
    private readonly userId: string,
    private readonly now: () => number = Date.now,
  ) {}

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): ChatSnapshot => this.snapshot;

  start(): void {
    this.stopped = false;
    this.patch({ connected: this.realtime.connected });
    void this.load();
    this.unsubscribe = this.realtime.subscribe({
      onFrame: (frame) => this.set(applyFrame(this.state, frame, this.conversationId)),
      onOpen: (reconnect) => {
        this.patch({ connected: true });
        if (reconnect) void this.catchUp();
      },
      onClose: () => this.patch({ connected: false }),
    });
  }

  stop(): void {
    this.stopped = true;
    this.unsubscribe?.();
    this.unsubscribe = null;
    if (this.typingTimer) clearTimeout(this.typingTimer);
  }

  load = async (): Promise<void> => {
    try {
      const page = await this.api.listMessages(this.conversationId, { limit: PAGE });
      if (this.stopped) return;
      this.set(setPage(this.state, page), { error: null, loading: false });
    } catch (error) {
      if (!this.stopped) this.patch({ error, loading: false });
    }
  };

  // loadOlder fetches the page before the oldest one on screen.
  loadOlder = async (): Promise<void> => {
    const before = this.state.nextBefore;
    if (!before || this.snapshot.loadingOlder) return;
    this.patch({ loadingOlder: true });
    try {
      const page = await this.api.listMessages(this.conversationId, { before, limit: PAGE });
      if (this.stopped) return;
      this.set(addOlder(this.state, page), { loadingOlder: false });
    } catch (error) {
      if (!this.stopped) this.patch({ error, loadingOlder: false });
    }
  };

  // catchUp asks for everything after the newest message we have. The
  // socket is a hint; the hub is the record.
  private async catchUp(): Promise<void> {
    let after = newestServerId(this.state);
    if (!after) return this.load();
    try {
      for (let pages = 0; after && pages < 10; pages++) {
        const page = await this.api.listMessages(this.conversationId, { after, limit: 100 });
        if (this.stopped) return;
        let state = this.state;
        for (const m of page.messages) state = upsert(state, m);
        this.set(state);
        after = page.next_after;
      }
    } catch (error) {
      if (!this.stopped) this.patch({ error });
    }
  }

  // send shows the message at once and tells the hub. A failure leaves it
  // on screen, marked, for retry.
  send = async (req: SendRequest): Promise<void> => {
    const key = newIdempotencyKey();
    const local: ChatMessage = {
      id: `local-${key}`,
      localKey: key,
      conversation_id: this.conversationId,
      sender: { kind: 'user', id: this.userId },
      body: req.action
        ? {
            text: req.action.label,
            action: { button_id: req.action.button_id, source_message_id: req.action.source_message_id },
          }
        : { text: req.text },
      reply_to: req.replyTo
        ? {
            id: req.replyTo.id,
            sender_kind: req.replyTo.sender.kind,
            text_preview: (req.replyTo.body.text ?? '').slice(0, 120),
          }
        : null,
      status: 'complete',
      truncated: false,
      delivery_status: 'pending',
      created_at: new Date(this.now()).toISOString(),
    };
    this.pending.set(key, req);
    this.set(addLocal(this.state, local));
    await this.deliver(key, req);
  };

  // retry sends a failed message again, with the same key, so the hub
  // creates it once however many times this is pressed.
  retry = async (key: string): Promise<void> => {
    const req = this.pending.get(key);
    if (!req) return;
    this.set(retryLocal(this.state, key));
    await this.deliver(key, req);
  };

  private async deliver(key: string, req: SendRequest): Promise<void> {
    try {
      const m = await this.api.sendMessage(
        this.conversationId,
        {
          text: req.action ? undefined : req.text,
          action: req.action
            ? { button_id: req.action.button_id, source_message_id: req.action.source_message_id }
            : undefined,
          reply_to: req.replyTo?.id,
        },
        key,
      );
      if (this.stopped) return;
      this.pending.delete(key);
      this.set(confirmLocal(this.state, key, m));
    } catch {
      if (!this.stopped) this.set(failLocal(this.state, key));
    }
  }

  private set(state: ChatState, extra: Partial<ChatSnapshot> = {}): void {
    this.state = state;
    this.armTyping();
    this.patch({
      messages: state.messages,
      typing: isTyping(state, this.now()),
      hasOlder: state.nextBefore !== null,
      quickReplies: quickReplies(state),
      ...extra,
    });
  }

  // The typing indicator ends on its own; a timer re-reads the state when
  // it is due to.
  private armTyping(): void {
    if (this.typingTimer) clearTimeout(this.typingTimer);
    this.typingTimer = null;
    const until = this.state.typingUntil;
    if (until === null || until <= this.now()) return;
    this.typingTimer = setTimeout(() => this.set(this.state), until - this.now() + 10);
  }

  private patch(extra: Partial<ChatSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...extra };
    for (const l of this.listeners) l();
  }
}
