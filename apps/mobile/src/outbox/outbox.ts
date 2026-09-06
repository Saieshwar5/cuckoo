import { NetworkError, type Api } from '../api/client';
import type { Message } from '../api/types';
import { keys, type Cache } from '../cache/cache';
import type { PickedFile } from '../media/pick';
import type { Realtime } from '../realtime/realtime';

// NetworkBack is how the outbox is told the network has returned: given
// a listener, it calls it each time that happens and returns how to stop.
// It is passed in rather than imported so the outbox knows nothing about
// the platform, and a test can say "the network is back" by calling it.
export type NetworkBack = (listener: () => void) => () => void;

// Sends that have not reached the hub yet.
//
// A message is written here before anything else happens to it, so a send
// typed with no signal is not lost when the app is closed. Whenever the
// connection is up the queue drains in order, and because every send
// already carries an idempotency key, one that was halfway through when the
// app died is sent again on the next open and lands exactly once. WhatsApp
// shows this as a small clock; so does Cuckoo.

// What one send needs, kept in the words the screen used rather than the
// wire's, so a message typed offline is drawn on the next open the same as
// it was drawn when typed.
export interface OutboxRequest {
  text?: string;
  files?: PickedFile[];
  action?: { button_id: string; source_message_id: string; label: string };
  replyTo?: { id: string; sender_kind: 'user' | 'agent'; text_preview: string };
}

export interface OutboxItem {
  // The idempotency key. It is also the local message's identity.
  key: string;
  conversationId: string;
  request: OutboxRequest;
  // Media ids for the files already uploaded, in order, so a retry never
  // uploads the same photo twice.
  uploaded: string[];
  // queued: will go when the hub can be reached. failed: the hub refused
  // it, and only a person tapping retry sends it again.
  state: 'queued' | 'failed';
  createdAt: string;
}

export type OutboxEvent =
  | { type: 'sent'; item: OutboxItem; message: Message }
  | { type: 'failed'; item: OutboxItem; error: unknown }
  | { type: 'queued'; item: OutboxItem };

// How long to wait before trying again when the hub could not be reached
// and the socket has not said it is back. The socket coming back is the
// real signal; this is the belt to that pair of braces.
const RETRY_MS = [5_000, 15_000, 60_000];

export class Outbox {
  private items: OutboxItem[] = [];
  private listeners = new Set<(e: OutboxEvent) => void>();
  private draining = false;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private attempt = 0;
  private loaded: Promise<void>;
  private unsubscribe: (() => void) | null = null;
  private stopListening: (() => void) | null = null;

  constructor(
    private readonly api: Api,
    private readonly cache: Cache,
    private readonly realtime: Realtime,
    private readonly userId: string,
    private readonly networkBack: NetworkBack = () => () => {},
  ) {
    this.loaded = this.cache.get<OutboxItem[]>(keys.outbox(userId)).then((saved) => {
      if (saved?.length) this.items = saved;
    });
  }

  // start drains what was saved and drains again whenever the connection
  // comes back.
  start(): void {
    void this.loaded.then(() => this.drain());
    this.unsubscribe = this.realtime.subscribe({
      onFrame: () => {},
      onOpen: () => void this.drain(),
    });
    this.stopListening = this.networkBack(() => {
      this.attempt = 0;
      void this.drain();
    });
  }

  stop(): void {
    this.unsubscribe?.();
    this.unsubscribe = null;
    this.stopListening?.();
    this.stopListening = null;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
  }

  subscribe(listener: (e: OutboxEvent) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  // pending is what has not gone yet, for a chat opening to draw.
  pending(conversationId: string): OutboxItem[] {
    return this.items.filter((i) => i.conversationId === conversationId);
  }

  // ready resolves once what was saved has been read, so a chat opening
  // right after the app does can ask for its pending sends.
  ready(): Promise<void> {
    return this.loaded;
  }

  async enqueue(key: string, conversationId: string, request: OutboxRequest): Promise<void> {
    await this.loaded;
    const item: OutboxItem = {
      key,
      conversationId,
      request,
      uploaded: [],
      state: 'queued',
      createdAt: new Date().toISOString(),
    };
    this.items = [...this.items, item];
    await this.persist();
    this.emit({ type: 'queued', item });
    // Resolves once this send has been attempted, or found the hub
    // unreachable: a caller that awaits a send learns what became of it.
    await this.drain();
  }

  // retry puts a refused send back in the queue.
  async retry(key: string): Promise<void> {
    await this.loaded;
    const item = this.items.find((i) => i.key === key);
    if (!item) return;
    item.state = 'queued';
    await this.persist();
    this.emit({ type: 'queued', item });
    await this.drain();
  }

  // drain sends what is queued, oldest first, one at a time. It stops at the
  // first send the hub could not be reached for: the rest would fail the
  // same way, and order matters more than speed.
  async drain(): Promise<void> {
    if (this.draining) return;
    this.draining = true;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    try {
      // The queue is re-read each time round, so a send added while an
      // earlier one was going out is not missed until the next trigger.
      for (;;) {
        const item = this.items.find((i) => i.state === 'queued');
        if (!item) break;
        const outcome = await this.send(item);
        if (outcome === 'unreachable') {
          this.scheduleRetry();
          return;
        }
      }
      this.attempt = 0;
    } finally {
      this.draining = false;
    }
  }

  private async send(item: OutboxItem): Promise<'sent' | 'failed' | 'unreachable'> {
    try {
      // The files go first and the message names them; what has already
      // gone is remembered, so a retry finishes rather than restarts.
      for (const file of (item.request.files ?? []).slice(item.uploaded.length)) {
        const media = await this.api.uploadMedia({
          uri: file.uri,
          name: file.name,
          mimeType: file.mimeType,
          durationMs: file.durationMs,
          waveform: file.waveform,
          audio: file.kind === 'audio',
        });
        item.uploaded = [...item.uploaded, media.id];
        await this.persist();
      }
      const { request } = item;
      const message = await this.api.sendMessage(
        item.conversationId,
        {
          text: request.action ? undefined : request.text,
          attachments: item.uploaded.length ? item.uploaded : undefined,
          action: request.action
            ? { button_id: request.action.button_id, source_message_id: request.action.source_message_id }
            : undefined,
          reply_to: request.replyTo?.id,
        },
        item.key,
      );
      this.items = this.items.filter((i) => i.key !== item.key);
      await this.persist();
      this.emit({ type: 'sent', item, message });
      return 'sent';
    } catch (error) {
      if (error instanceof NetworkError) return 'unreachable';
      // The hub answered and said no. That is not going to change on its
      // own, so it waits for a person.
      item.state = 'failed';
      await this.persist();
      this.emit({ type: 'failed', item, error });
      return 'failed';
    }
  }

  private scheduleRetry(): void {
    if (this.timer) return;
    const delay = RETRY_MS[Math.min(this.attempt, RETRY_MS.length - 1)] as number;
    this.attempt++;
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.drain();
    }, delay);
  }

  private persist(): Promise<void> {
    return this.cache.set(keys.outbox(this.userId), this.items);
  }

  private emit(e: OutboxEvent): void {
    for (const l of this.listeners) l(e);
  }
}
