import type { Api } from '../api/client';
import type { Conversation } from '../api/types';
import type { Realtime } from '../realtime/realtime';
import { applyFrame, empty, setConversations, type ChatsState } from './store';

export interface ChatsSnapshot {
  conversations: Conversation[];
  loading: boolean;
  error: unknown;
  connected: boolean;
}

// ChatsController is the chat list as a thing outside React: loaded from the
// hub, kept live by the session's connection, reloaded whenever that
// connection comes back because whatever happened while it was down is in
// the hub and not in any frame. A screen subscribes to snapshots; nothing
// here knows about screens.
export class ChatsController {
  private state: ChatsState = empty;
  private snapshot: ChatsSnapshot = { conversations: [], loading: true, error: null, connected: false };
  private listeners = new Set<() => void>();
  private unsubscribe: (() => void) | null = null;
  private stopped = false;

  constructor(
    private readonly api: Api,
    private readonly realtime: Realtime,
  ) {}

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): ChatsSnapshot => this.snapshot;

  // start loads the list and listens for frames. Call stop when done.
  start(): void {
    this.stopped = false;
    this.patch({ connected: this.realtime.connected });
    void this.refresh();
    this.unsubscribe = this.realtime.subscribe({
      onFrame: (frame) => this.set(applyFrame(this.state, frame)),
      onOpen: (reconnect) => {
        this.patch({ connected: true });
        if (reconnect) void this.refresh();
      },
      onClose: () => this.patch({ connected: false }),
    });
  }

  stop(): void {
    this.stopped = true;
    this.unsubscribe?.();
    this.unsubscribe = null;
  }

  refresh = async (): Promise<void> => {
    try {
      const list = await this.api.listConversations();
      if (this.stopped) return;
      this.set(setConversations(this.state, list), { error: null, loading: false });
    } catch (error) {
      if (!this.stopped) this.patch({ error, loading: false });
    }
  };

  private set(state: ChatsState, extra: Partial<ChatsSnapshot> = {}): void {
    this.state = state;
    this.patch({ conversations: state.conversations, ...extra });
  }

  private patch(extra: Partial<ChatsSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...extra };
    for (const l of this.listeners) l();
  }
}
