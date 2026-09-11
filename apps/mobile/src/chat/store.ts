import type { DeliveryStatus, Frame, Message, Page } from '../api/types';
import { activityOf, isLive, type Activity } from './activity';

// One conversation's messages, and how live frames and our own sends change
// them. Pure, so it is tested without a screen.

// A message as the screen holds it: the hub's, or one of ours the hub has
// not confirmed yet, which carries the key it was sent with so a retry is
// the same send.
export interface ChatMessage extends Message {
  localKey?: string;
}

export interface ChatState {
  // Newest first, the order an inverted list wants.
  messages: ChatMessage[];
  // Cursor for the page before the oldest one loaded; null when history
  // is fully loaded.
  nextBefore: string | null;
  // The hub said the conversation is older than what it still holds.
  trimmed: boolean;
  // What the agent last said it was doing, until when.
  activity: Activity | null;
  // The last moment the agent showed any sign of life short of a message:
  // when its latest activity ran out, or was ended.
  lastSignal: number;
  // When this person last pressed stop here, on this device.
  stoppedAt: number | null;
}

export const empty: ChatState = {
  messages: [],
  nextBefore: null,
  trimmed: false,
  activity: null,
  lastSignal: 0,
  stoppedAt: null,
};

// How long a person's message may sit, received and unanswered, before the
// chat says so. Long enough for any agent that is merely slow to have at
// least said it is thinking.
export const NO_REPLY_AFTER_MS = 30_000;

// newer orders two messages. At the same instant, one of ours still waiting
// for the hub is the newer: it was typed on this device just now.
function newer(a: ChatMessage, b: ChatMessage): boolean {
  if (a.created_at !== b.created_at) return a.created_at > b.created_at;
  if (!!a.localKey !== !!b.localKey) return !!a.localKey;
  return a.id > b.id;
}

function insert(list: ChatMessage[], m: ChatMessage): ChatMessage[] {
  const i = list.findIndex((x) => !newer(x, m));
  const next = list.slice();
  next.splice(i < 0 ? list.length : i, 0, m);
  return next;
}

function patch(list: ChatMessage[], id: string, fn: (m: ChatMessage) => ChatMessage): ChatMessage[] {
  const i = list.findIndex((m) => m.id === id);
  if (i < 0) return list;
  const next = list.slice();
  next[i] = fn(list[i] as ChatMessage);
  return next;
}

// setPage replaces everything with the latest page of history.
export function setPage(state: ChatState, page: Page): ChatState {
  return { ...state, messages: page.messages, nextBefore: page.next_before, trimmed: page.trimmed };
}

// addOlder appends a page of older history.
export function addOlder(state: ChatState, page: Page): ChatState {
  const known = new Set(state.messages.map((m) => m.id));
  const older = page.messages.filter((m) => !known.has(m.id));
  return {
    ...state,
    messages: [...state.messages, ...older],
    nextBefore: page.next_before,
    trimmed: page.trimmed,
  };
}

// historyTrimmed says whether the top of the chat is where the hub stopped
// keeping, rather than where the conversation began. Only once the oldest
// page is on screen: while there is more to load, nothing has ended yet.
export function historyTrimmed(state: ChatState): boolean {
  return state.nextBefore === null && state.trimmed;
}

// upsert puts a message from the hub where it belongs. A message from us
// that we sent from this device replaces the unconfirmed copy that was
// waiting for it. Anything from the agent means whatever it said it was
// doing has come to this.
export function upsert(state: ChatState, m: Message): ChatState {
  const activity = m.sender.kind === 'agent' ? null : state.activity;
  if (state.messages.some((x) => x.id === m.id)) {
    return { ...state, activity, messages: patch(state.messages, m.id, () => m) };
  }
  let list = state.messages;
  if (m.sender.kind === 'user') {
    const j = list.findIndex((x) => x.localKey && x.delivery_status !== 'failed' && sameSend(x, m));
    if (j >= 0) list = list.filter((_, k) => k !== j);
  }
  return { ...state, activity, messages: insert(list, m) };
}

function sameSend(local: ChatMessage, m: Message): boolean {
  if (local.body.action || m.body.action) {
    return local.body.action?.button_id === m.body.action?.button_id;
  }
  return (local.body.text ?? '') === (m.body.text ?? '');
}

export function appendDelta(state: ChatState, id: string, text: string): ChatState {
  return {
    ...state,
    messages: patch(state.messages, id, (m) => ({
      ...m,
      body: { ...m.body, text: (m.body.text ?? '') + text },
    })),
  };
}

export function setDelivery(state: ChatState, id: string, status: DeliveryStatus): ChatState {
  return { ...state, messages: patch(state.messages, id, (m) => ({ ...m, delivery_status: status })) };
}

export function setActivity(state: ChatState, data: Extract<Frame, { type: 'activity' }>['data']): ChatState {
  return { ...state, activity: activityOf(data), lastSignal: Date.parse(data.expires_at) };
}

// markStopped is this person pressing stop: whatever the agent was doing is
// over as far as this screen is concerned, before the hub has even said so.
export function markStopped(state: ChatState, now: number): ChatState {
  return { ...state, activity: null, stoppedAt: now };
}

// addLocal puts our own send on screen before the hub has answered.
export function addLocal(state: ChatState, m: ChatMessage): ChatState {
  return { ...state, messages: [m, ...state.messages] };
}

// confirmLocal swaps the unconfirmed copy for what the hub created. If the
// hub's copy already arrived over the socket, the local one just goes.
export function confirmLocal(state: ChatState, key: string, m: Message): ChatState {
  const without = { ...state, messages: state.messages.filter((x) => x.localKey !== key) };
  return upsert(without, m);
}

export function failLocal(state: ChatState, key: string): ChatState {
  return {
    ...state,
    messages: state.messages.map((m) =>
      m.localKey === key ? { ...m, delivery_status: 'failed' as const } : m,
    ),
  };
}

export function retryLocal(state: ChatState, key: string): ChatState {
  return {
    ...state,
    messages: state.messages.map((m) =>
      m.localKey === key ? { ...m, delivery_status: 'pending' as const } : m,
    ),
  };
}

// applyFrame folds one live frame into the conversation. Frames for other
// conversations are not ours to handle.
export function applyFrame(state: ChatState, frame: Frame, conversationId: string): ChatState {
  // Announcements about an agent are the chat list's and the header's
  // business, not the thread's.
  if (frame.type === 'ready' || frame.type === 'agent.status') return state;
  if (frame.data.conversation_id !== conversationId) return state;
  switch (frame.type) {
    case 'message.created':
    case 'message.started':
    case 'message.completed':
      return upsert(state, frame.data.message);
    case 'message.delta':
      return appendDelta(state, frame.data.message_id, frame.data.text);
    case 'delivery.updated':
      return setDelivery(state, frame.data.message_id, frame.data.delivery_status);
    case 'activity':
      return setActivity(state, frame.data);
    case 'conversation.read':
      // The badge is the chat list's; an open chat is being read already.
      return state;
    case 'schedule.changed':
      // The schedules screen's business; a message it sends arrives as one.
      return state;
  }
}

// newestServerId is the cursor for catching up: the newest message the hub
// gave us, skipping our own unconfirmed ones.
export function newestServerId(state: ChatState): string | null {
  return state.messages.find((m) => !m.localKey)?.id ?? null;
}

// quickReplies are the agent's suggested answers to its latest message,
// offered only while that message is the last word in the conversation.
export function quickReplies(state: ChatState): string[] {
  const last = state.messages[0];
  if (!last || last.sender.kind !== 'agent' || last.status !== 'complete') return [];
  return (last.body.quick_replies ?? []).map((q) => q.label);
}

// liveActivity is what the agent is doing now, or null once it has run out.
export function liveActivity(state: ChatState, now: number): Activity | null {
  return isLive(state.activity, now) ? state.activity : null;
}

// writing says an agent's reply is appearing, word by word.
export function writing(state: ChatState): boolean {
  return state.messages.some((m) => m.status === 'streaming' && m.sender.kind === 'agent');
}

// noReplyDue is when "no reply yet" becomes true, or null when it cannot:
// the person spoke last, the agent's backend has the message, and since
// then there has been no word and no sign of work. A message still on its
// way or refused has its own marks, and so does a stop the person pressed
// after it.
export function noReplyDue(state: ChatState): number | null {
  const last = state.messages[0];
  if (!last || last.sender.kind !== 'user' || last.localKey) return null;
  if (last.delivery_status !== 'delivered') return null;
  const said = Date.parse(last.created_at);
  if (state.stoppedAt !== null && state.stoppedAt >= said) return null;
  return Math.max(said, state.lastSignal) + NO_REPLY_AFTER_MS;
}

// lastWords is what the person last said, when it can simply be said
// again: plain words, not a tap or a file.
export function lastWords(state: ChatState): string | null {
  const last = state.messages[0];
  if (!last || last.sender.kind !== 'user' || last.body.action || last.body.attachments?.length) return null;
  return last.body.text?.trim() || null;
}

// removeMessage takes one message off the screen: the person deleted it
// for themselves. One of ours still waiting for the hub has no id to give
// the hub, so it is not offered for this.
export function removeMessage(state: ChatState, id: string): ChatState {
  if (!state.messages.some((m) => m.id === id)) return state;
  return { ...state, messages: state.messages.filter((m) => m.id !== id) };
}

// clearMessages empties the chat: everything so far is out of sight, and
// there is nothing older to page to. What the hub swept before that is
// beside the point now; the person chose to start again.
export function clearMessages(state: ChatState): ChatState {
  return { ...state, messages: [], nextBefore: null, trimmed: false };
}

// unseenSince counts what others said after a message the person was
// looking at when they scrolled away. Ids are time-ordered, so newer means
// greater; our own sends do not count — the person wrote them.
export function unseenSince(messages: ChatMessage[], sinceId: string | null): number {
  if (!sinceId) return 0;
  let n = 0;
  for (const m of messages) {
    if (m.id <= sinceId) break;
    if (m.sender.kind !== 'user') n += 1;
  }
  return n;
}
