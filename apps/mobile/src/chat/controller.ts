import { newIdempotencyKey, type Api } from '../api/client';
import type { Attachment, Message } from '../api/types';
import { keys, MemoryCache, MESSAGES_KEPT, type Cache } from '../cache/cache';
import type { PickedFile } from '../media/pick';
import { Outbox, type OutboxEvent, type OutboxItem } from '../outbox/outbox';
import type { Realtime } from '../realtime/realtime';
import type { Activity } from './activity';
import {
  addLocal,
  addOlder,
  applyFrame,
  confirmLocal,
  empty,
  failLocal,
  historyTrimmed,
  lastWords,
  liveActivity,
  markStopped,
  newestServerId,
  noReplyDue,
  quickReplies,
  removeMessage,
  retryLocal,
  setPage,
  type ChatMessage,
  type ChatState,
  upsert,
  writing,
} from './store';

export interface ChatSnapshot {
  messages: ChatMessage[];
  // What the agent says it is doing, while it lasts.
  activity: Activity | null;
  // A reply is appearing word by word.
  writing: boolean;
  // Either: the agent is busy here, and there is something to stop.
  busy: boolean;
  // The person's last message reached the agent a while ago, and nothing
  // has come back — not a word, not a sign of work.
  noReply: boolean;
  // What the person last said, if it can be said again as it was.
  lastWords: string | null;
  loading: boolean;
  loadingOlder: boolean;
  hasOlder: boolean;
  // The oldest message on screen is the oldest the hub still has, and
  // there were older ones once.
  trimmed: boolean;
  connected: boolean;
  error: unknown;
  quickReplies: string[];
}

// What a screen asks to send: words, or a tap on a button an agent
// offered, either quoting an earlier message.
export interface SendRequest {
  text?: string;
  // Files picked on this device, not yet on the hub.
  files?: PickedFile[];
  action?: { button_id: string; source_message_id: string; label: string };
  replyTo?: Message;
}

const PAGE = 50;

// How long after the newest message arrives it is marked read. A stream
// brings a message and then its words; one call for the message is enough.
const READ_DELAY_MS = 500;

// How long after the last change a chat's messages are written down. A
// stream delivers a piece of text many times a second; writing five
// hundred messages on each would be most of what the phone did.
const WRITE_DELAY_MS = 400;

// What a chat needs from the rest of the session to remember and to send:
// where to remember, whose it is, and the queue sends go through.
export interface ChatDeps {
  cache: Cache;
  userId: string;
  outbox: Outbox;
}

// A remembered chat: the messages the hub gave us and where older history
// continues. Our own unsent messages are not here; the outbox has those.
interface Remembered {
  messages: Message[];
  nextBefore: string | null;
  // Written down since retention arrived; a chat remembered before then
  // has no answer, and no answer means no note.
  trimmed?: boolean;
}

// ChatController is one open conversation as a thing outside React: its
// history remembered from last time and loaded from the hub, kept live by
// the session's connection, our own sends shown at once and carried by the
// outbox until the hub has them.
export class ChatController {
  private state: ChatState = empty;
  private snapshot: ChatSnapshot = {
    messages: [],
    activity: null,
    writing: false,
    busy: false,
    noReply: false,
    lastWords: null,
    loading: true,
    loadingOlder: false,
    hasOlder: false,
    trimmed: false,
    connected: false,
    error: null,
    quickReplies: [],
  };
  private listeners = new Set<() => void>();
  private unsubscribe: (() => void) | null = null;
  private unsubscribeOutbox: (() => void) | null = null;
  // Wakes the snapshot when something shown runs out on its own: an
  // activity expiring, or the wait for a reply growing long.
  private wakeTimer: ReturnType<typeof setTimeout> | null = null;
  private readTimer: ReturnType<typeof setTimeout> | null = null;
  // Whether this person can see the chat right now: on screen, and the app
  // in front. Only then is anything marked read.
  private visible = false;
  private readUpTo: string | null = null;
  private writeTimer: ReturnType<typeof setTimeout> | null = null;
  private stopped = false;
  private readonly cache: Cache;
  private readonly cacheKey: string;
  private readonly outbox: Outbox;

  constructor(
    private readonly api: Api,
    private readonly realtime: Realtime,
    private readonly conversationId: string,
    private readonly userId: string,
    private readonly now: () => number = Date.now,
    deps?: ChatDeps,
  ) {
    // Without a session's cache and outbox — a test of something else —
    // the chat remembers nothing and sends through a queue of its own,
    // which behaves as sending always did.
    this.cache = deps?.cache ?? new MemoryCache();
    this.cacheKey = keys.messages(deps?.userId ?? userId, conversationId);
    this.outbox = deps?.outbox ?? new Outbox(api, this.cache, realtime, userId);
    if (!deps) this.outbox.start();
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getSnapshot = (): ChatSnapshot => this.snapshot;

  start(): void {
    this.stopped = false;
    this.patch({ connected: this.realtime.connected });
    void this.recall().then(() => this.load());
    this.unsubscribeOutbox = this.outbox.subscribe((e) => this.onOutbox(e));
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
    this.unsubscribeOutbox?.();
    this.unsubscribeOutbox = null;
    if (this.wakeTimer) clearTimeout(this.wakeTimer);
    if (this.readTimer) clearTimeout(this.readTimer);
    this.readTimer = null;
    if (this.writeTimer) {
      clearTimeout(this.writeTimer);
      this.writeTimer = null;
      this.remember();
    }
  }

  // recall puts the chat on screen as it was last time — the hub's
  // messages, then our own that never left — before the hub is asked.
  private async recall(): Promise<void> {
    const saved = await this.cache.get<Remembered>(this.cacheKey);
    await this.outbox.ready();
    if (this.stopped) return;
    let state = this.state;
    if (saved?.messages.length && !state.messages.length) {
      state = setPage(state, {
        messages: saved.messages,
        next_before: saved.nextBefore,
        next_after: null,
        trimmed: saved.trimmed ?? false,
      });
    }
    for (const item of this.outbox.pending(this.conversationId)) {
      if (!state.messages.some((m) => m.localKey === item.key)) {
        state = addLocal(state, this.localMessage(item));
      }
    }
    if (state !== this.state) this.set(state, { loading: false });
  }

  load = async (): Promise<void> => {
    try {
      const page = await this.api.listMessages(this.conversationId, { limit: PAGE });
      if (this.stopped) return;
      // The hub's page replaces what was remembered; what we have not sent
      // yet is ours and stays.
      const unsent = this.state.messages.filter((m) => m.localKey);
      let state = setPage(this.state, page);
      for (const m of unsent.reverse()) state = addLocal(state, m);
      this.set(state, { error: null, loading: false });
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

  // send shows the message at once and hands it to the outbox, which
  // carries it to the hub now or whenever the hub can next be reached.
  send = async (req: SendRequest): Promise<void> => {
    const key = newIdempotencyKey();
    const item: OutboxItem = {
      key,
      conversationId: this.conversationId,
      request: {
        text: req.text,
        files: req.files,
        action: req.action,
        replyTo: req.replyTo
          ? {
              id: req.replyTo.id,
              sender_kind: req.replyTo.sender.kind,
              text_preview: (req.replyTo.body.text ?? '').slice(0, 120),
            }
          : undefined,
      },
      uploaded: [],
      state: 'queued',
      createdAt: new Date(this.now()).toISOString(),
    };
    this.set(addLocal(this.state, this.localMessage(item)));
    await this.outbox.enqueue(key, this.conversationId, item.request);
  };

  // stopAgent is the person pressing stop. The screen stops showing work at
  // once; the hub ends whatever reply was being written and tells the agent,
  // and the replies it ended come back as they now stand.
  stopAgent = async (): Promise<void> => {
    this.set(markStopped(this.state, this.now()));
    try {
      const ended = await this.api.stopConversation(this.conversationId);
      if (this.stopped) return;
      let state = this.state;
      for (const m of ended) state = upsert(state, m);
      this.set(state);
    } catch (error) {
      if (!this.stopped) this.patch({ error });
    }
  };

  // sendAgain says the person's last words again, for an agent that never
  // answered them.
  sendAgain = async (): Promise<void> => {
    const words = lastWords(this.state);
    if (words) await this.send({ text: words });
  };

  // setVisible tells the chat whether the person can see it. While they
  // can, whatever arrives is read.
  setVisible = (visible: boolean): void => {
    this.visible = visible;
    if (visible) this.scheduleRead();
  };

  // scheduleRead marks the newest message read, a moment after it lands.
  // A mark that fails is simply tried again with the next message.
  private scheduleRead(): void {
    if (!this.visible || this.readTimer) return;
    const newest = newestServerId(this.state);
    if (!newest || (this.readUpTo !== null && newest <= this.readUpTo)) return;
    this.readTimer = setTimeout(() => {
      this.readTimer = null;
      const id = newestServerId(this.state);
      if (!this.visible || !id || (this.readUpTo !== null && id <= this.readUpTo)) return;
      this.readUpTo = id;
      this.api.markRead(this.conversationId, id).catch(() => {
        this.readUpTo = null;
      });
    }, READ_DELAY_MS);
  }

  // deleteForMe takes a message out of this person's view, here and on the
  // hub. The hub is asked first: a delete that did not reach it would come
  // back on the next load, which is worse than a moment's wait.
  deleteForMe = async (id: string): Promise<void> => {
    await this.api.deleteMessageForMe(this.conversationId, id);
    this.set(removeMessage(this.state, id));
  };

  // retry puts a message the hub refused back in the queue, with the same
  // key, so the hub creates it once however many times this is pressed.
  retry = async (key: string): Promise<void> => {
    this.set(retryLocal(this.state, key));
    await this.outbox.retry(key);
  };

  // onOutbox is the queue reporting on one of our sends: gone, or refused.
  // A send the hub could not be reached for reports nothing; it stays
  // pending, which is the truth of it.
  private onOutbox(e: OutboxEvent): void {
    if (e.item.conversationId !== this.conversationId || this.stopped) return;
    switch (e.type) {
      case 'sent':
        this.set(confirmLocal(this.state, e.item.key, e.message));
        break;
      case 'failed':
        this.set(failLocal(this.state, e.item.key));
        break;
      case 'queued':
        break;
    }
  }

  // localMessage is how one of our own sends looks before the hub has it:
  // the bubble drawn from what was typed and picked, on this device.
  private localMessage(item: OutboxItem): ChatMessage {
    const { request } = item;
    return {
      id: `local-${item.key}`,
      localKey: item.key,
      conversation_id: this.conversationId,
      sender: { kind: 'user', id: this.userId },
      body: request.action
        ? {
            text: request.action.label,
            action: {
              button_id: request.action.button_id,
              source_message_id: request.action.source_message_id,
            },
          }
        : { text: request.text, attachments: localAttachments(request.files) },
      reply_to: request.replyTo ?? null,
      status: 'complete',
      truncated: false,
      delivery_status: item.state === 'failed' ? 'failed' : 'pending',
      created_at: item.createdAt,
    };
  }

  // remember writes the hub's messages down, a little after the last
  // change. Our own unsent ones are the outbox's to remember.
  private remember(): void {
    const messages = this.state.messages.filter((m) => !m.localKey).slice(0, MESSAGES_KEPT);
    void this.cache.set(this.cacheKey, {
      messages,
      nextBefore: this.state.nextBefore,
      trimmed: this.state.trimmed,
    } satisfies Remembered);
  }

  private set(state: ChatState, extra: Partial<ChatSnapshot> = {}): void {
    this.state = state;
    this.armWake();
    this.scheduleRead();
    if (this.writeTimer) clearTimeout(this.writeTimer);
    this.writeTimer = setTimeout(() => {
      this.writeTimer = null;
      this.remember();
    }, WRITE_DELAY_MS);
    const now = this.now();
    const activity = liveActivity(state, now);
    const busy = !!activity || writing(state);
    const due = noReplyDue(state);
    this.patch({
      messages: state.messages,
      activity,
      writing: writing(state),
      busy,
      noReply: due !== null && due <= now && !busy,
      lastWords: lastWords(state),
      hasOlder: state.nextBefore !== null,
      trimmed: historyTrimmed(state),
      quickReplies: quickReplies(state),
      ...extra,
    });
  }

  // An activity ends on its own, and a wait turns into "no reply yet" on
  // its own; a timer re-reads the state at the first of the two.
  private armWake(): void {
    if (this.wakeTimer) clearTimeout(this.wakeTimer);
    this.wakeTimer = null;
    const now = this.now();
    const due = [this.state.activity?.until, noReplyDue(this.state)].filter(
      (at): at is number => typeof at === 'number' && at > now,
    );
    if (!due.length) return;
    this.wakeTimer = setTimeout(() => this.set(this.state), Math.min(...due) - now + 10);
  }

  private patch(extra: Partial<ChatSnapshot>): void {
    this.snapshot = { ...this.snapshot, ...extra };
    for (const l of this.listeners) l();
  }
}

// localAttachments is what our own bubble shows while the files are still
// going up: the pictures on this device, drawn from their own paths, with
// no media id yet because the hub has not seen them.
function localAttachments(files: PickedFile[] | undefined): Attachment[] | undefined {
  if (!files?.length) return undefined;
  return files.map((f) => ({
    media_id: '',
    kind: f.kind,
    mime_type: f.mimeType,
    byte_size: f.byteSize,
    file_name: f.name,
    width: f.width,
    height: f.height,
    duration_ms: f.durationMs,
    waveform: f.waveform,
    local_uri: f.uri,
  }));
}
